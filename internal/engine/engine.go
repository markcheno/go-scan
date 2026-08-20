package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/markcheno/go-quote"
)

// DefaultConcurrency is how many tickers are fetched at once.
const DefaultConcurrency = 6

// Phase names reported through Options.Progress.
const (
	PhaseResolve   = "resolve"
	PhaseFetch     = "fetch"
	PhaseCompute   = "compute"
	PhaseTransform = "transform"
	PhaseWrite     = "write"
)

// Progress describes how far along a run is.
type Progress struct {
	Phase  string `json:"phase"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
	Ticker string `json:"ticker,omitempty"`
}

// TickerVerdict records the filter outcome for one ticker along with the row
// the filter was evaluated against.
type TickerVerdict struct {
	Ticker string            `json:"ticker"`
	Passed bool              `json:"passed"`
	Bars   int               `json:"bars"`
	Values map[string]string `json:"values"`
}

// TickerError records a per-ticker failure that did not stop the run.
type TickerError struct {
	Ticker  string `json:"ticker"`
	Message string `json:"message"`
}

// Result is everything a run produced.
type Result struct {
	Headers  []string        `json:"headers"`
	Rows     [][]string      `json:"rows"`
	Train    [][]string      `json:"train,omitempty"`
	Test     [][]string      `json:"test,omitempty"`
	Verdicts []TickerVerdict `json:"verdicts"`
	Errors   []TickerError   `json:"errors"`
	Files    []string        `json:"files"`
	Tickers  []string        `json:"tickers"`
	// Universe is the size of the resolved universe before MaxTickers was
	// applied, so a sampled preview can still report how big the real run is.
	Universe int           `json:"universe"`
	Elapsed  time.Duration `json:"elapsed"`
}

// Options control how a run executes. The zero value is a full run with the
// default fetcher.
type Options struct {
	// Fetcher supplies quote data. Defaults to a cacheless QuoteFetcher.
	Fetcher Fetcher
	// Lister resolves market names. Defaults to Fetcher when it implements
	// MarketLister, otherwise to go-quote directly.
	Lister MarketLister
	// Concurrency bounds simultaneous fetches. Defaults to DefaultConcurrency.
	Concurrency int
	// Progress, when set, is called as the run advances. It may be called
	// from multiple goroutines.
	Progress func(Progress)
	// Log, when set, receives human-readable status lines.
	Log func(string)
	// DryRun computes everything but writes no files.
	DryRun bool
	// MaxTickers limits how much of the universe is processed. 0 means all.
	MaxTickers int
	// MaxBars keeps only the most recent N bars per ticker. 0 means all.
	MaxBars int
}

func (o *Options) fetcher() Fetcher {
	if o.Fetcher != nil {
		return o.Fetcher
	}
	return NewQuoteFetcher(nil)
}

func (o *Options) lister() MarketLister {
	if o.Lister != nil {
		return o.Lister
	}
	if l, ok := o.fetcher().(MarketLister); ok {
		return l
	}
	return NewQuoteFetcher(nil)
}

func (o *Options) concurrency() int {
	if o.Concurrency > 0 {
		return o.Concurrency
	}
	return DefaultConcurrency
}

func (o *Options) progress(p Progress) {
	if o.Progress != nil {
		o.Progress(p)
	}
}

func (o *Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

// Run executes a scan end to end.
func Run(ctx context.Context, cfg *Config, opts Options) (*Result, error) {
	started := time.Now()

	if problems := Validate(cfg); HasErrors(problems) {
		return nil, FirstError(problems)
	}

	tickers, err := ResolveTickers(ctx, cfg, opts.lister())
	if err != nil {
		return nil, err
	}
	universe := len(tickers)
	if opts.MaxTickers > 0 && len(tickers) > opts.MaxTickers {
		tickers = tickers[:opts.MaxTickers]
	}

	columnMap, err := parseColumnSpecs(cfg.Columns.Items())
	if err != nil {
		return nil, err
	}
	headers := append(append([]string{}, BaseHeaders...), columnMap.Keys()...)

	result := &Result{Headers: headers, Tickers: tickers, Universe: universe}

	quotes := fetchAll(ctx, cfg, tickers, opts, result)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Each table carries its header in row 0 so the transforms below can be
	// applied to all three uniformly.
	allRows := [][]string{headers}
	trainRows := [][]string{headers}
	testRows := [][]string{headers}
	split := cfg.SplitPct > 0 && cfg.SplitPct < 1

	for i, q := range quotes {
		if q == nil {
			continue
		}
		opts.progress(Progress{Phase: PhaseCompute, Done: i + 1, Total: len(tickers), Ticker: tickers[i]})

		rows, verdict, err := processTicker(ctx, cfg, *q, columnMap, headers, opts.MaxBars)
		if err != nil {
			result.Errors = append(result.Errors, TickerError{Ticker: tickers[i], Message: err.Error()})
			opts.logf("skipping %s: %v", tickers[i], err)
			continue
		}
		result.Verdicts = append(result.Verdicts, verdict)
		if !verdict.Passed {
			opts.logf("excluding %s due to filter", tickers[i])
			continue
		}
		opts.logf("keeping %s (%d rows)", tickers[i], len(rows))
		allRows = append(allRows, rows...)

		// Split each ticker's own history, so every ticker contributes to both
		// sets rather than whole tickers landing in one or the other.
		if split {
			cut := int(float64(len(rows)) * cfg.SplitPct)
			trainRows = append(trainRows, rows[:cut]...)
			testRows = append(testRows, rows[cut:]...)
		}
	}

	opts.progress(Progress{Phase: PhaseTransform, Done: 0, Total: 1})
	transform := func(rows [][]string) [][]string {
		rows = dropColumns(rows, splitCommaList(cfg.DropColumns))
		if cfg.Pivot {
			rows = pivot(rows, "date", "symbol")
			rows = dropOtherTargets(rows, cfg.TargetColumn)
		}
		return rows
	}

	allRows = transform(allRows)
	result.Headers = allRows[0]
	result.Rows = allRows[1:]
	if split {
		result.Train = transform(trainRows)[1:]
		result.Test = transform(testRows)[1:]
	}
	opts.progress(Progress{Phase: PhaseTransform, Done: 1, Total: 1})

	if !opts.DryRun {
		files, err := writeOutputs(cfg, result, opts)
		result.Files = files
		if err != nil {
			return result, err
		}
	}

	result.Elapsed = time.Since(started)
	return result, nil
}

// ResolveTickers expands the market and ticker list into a deduplicated,
// uppercase-preserving symbol list.
func ResolveTickers(ctx context.Context, cfg *Config, lister MarketLister) ([]string, error) {
	var tickers []string

	if cfg.Market != "" {
		symbols, err := lister.Market(ctx, cfg.Market)
		if err != nil {
			return nil, err
		}
		tickers = append(tickers, symbols...)
	}

	// A single -tickers value may itself be comma separated.
	for _, item := range cfg.Tickers.Items() {
		for _, part := range strings.Split(item, ",") {
			if part = strings.TrimSpace(part); part != "" {
				tickers = append(tickers, part)
			}
		}
	}

	seen := make(map[string]bool, len(tickers))
	unique := tickers[:0]
	for _, ticker := range tickers {
		if !seen[ticker] {
			seen[ticker] = true
			unique = append(unique, ticker)
		}
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("no tickers specified")
	}
	return unique, nil
}

// fetchAll retrieves every ticker with bounded concurrency, preserving order.
// Per-ticker failures are recorded on result rather than aborting the run.
func fetchAll(ctx context.Context, cfg *Config, tickers []string, opts Options, result *Result) []*quote.Quote {
	quotes := make([]*quote.Quote, len(tickers))
	fetcher := opts.fetcher()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		done int
		sem  = make(chan struct{}, opts.concurrency())
	)

	opts.progress(Progress{Phase: PhaseFetch, Done: 0, Total: len(tickers)})
	for i, ticker := range tickers {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}
			q, err := fetcher.Fetch(ctx, cfg, ticker)

			mu.Lock()
			defer mu.Unlock()
			done++
			if err != nil {
				result.Errors = append(result.Errors, TickerError{Ticker: ticker, Message: err.Error()})
				opts.logf("failed to fetch %s: %v", ticker, err)
			} else {
				quotes[i] = &q
			}
			opts.progress(Progress{Phase: PhaseFetch, Done: done, Total: len(tickers), Ticker: ticker})
		}()
	}
	wg.Wait()

	// Keep error order deterministic regardless of completion order.
	slices.SortStableFunc(result.Errors, func(a, b TickerError) int {
		return slices.Index(tickers, a.Ticker) - slices.Index(tickers, b.Ticker)
	})
	return quotes
}

// processTicker computes the user columns for one quote, formats its rows and
// evaluates the filter against the last row.
func processTicker(ctx context.Context, cfg *Config, q quote.Quote, columnMap *OrderedMap, headers []string, maxBars int) ([][]string, TickerVerdict, error) {
	verdict := TickerVerdict{Ticker: q.Symbol, Bars: len(q.Date)}

	if cfg.Truncate >= len(q.Date) {
		return nil, verdict, fmt.Errorf("truncate of %d leaves no rows (only %d bars available)", cfg.Truncate, len(q.Date))
	}

	ev := NewEvaluator()
	if err := ev.BindQuote(q); err != nil {
		return nil, verdict, err
	}

	columns := NewOrderedMap()
	for _, name := range columnMap.Keys() {
		expr, _ := columnMap.Get(name)
		values, err := ev.Column(ctx, name, expr.(string))
		if err != nil {
			return nil, verdict, fmt.Errorf("column %s: %w", name, err)
		}
		if len(values) != len(q.Date) {
			return nil, verdict, fmt.Errorf("column %s produced %d values for %d bars", name, len(values), len(q.Date))
		}
		columns.Set(name, values)
	}

	start := cfg.Truncate
	if maxBars > 0 && len(q.Date)-start > maxBars {
		start = len(q.Date) - maxBars
	}

	layout := DateColumnLayout(cfg.Period)
	rows := make([][]string, 0, len(q.Date)-start)
	for i := start; i < len(q.Date); i++ {
		record := []string{
			q.Symbol,
			q.Date[i].Format(layout),
			fmt.Sprintf("%f", q.Open[i]),
			fmt.Sprintf("%f", q.High[i]),
			fmt.Sprintf("%f", q.Low[i]),
			fmt.Sprintf("%f", q.Close[i]),
			fmt.Sprintf("%f", q.Volume[i]),
		}
		for _, name := range columns.Keys() {
			values, _ := columns.Get(name)
			record = append(record, fmt.Sprintf("%f", values.([]float64)[i]))
		}
		rows = append(rows, record)
	}
	if len(rows) == 0 {
		return nil, verdict, fmt.Errorf("no rows after truncation")
	}

	last := rows[len(rows)-1]
	verdict.Values = make(map[string]string, len(headers))
	for i, name := range headers {
		if i < len(last) {
			verdict.Values[name] = last[i]
		}
	}

	passed, err := EvalFilterContext(ctx, cfg.Filter, headers, last)
	if err != nil {
		return nil, verdict, fmt.Errorf("filter: %w", err)
	}
	verdict.Passed = passed
	return rows, verdict, nil
}

// outTarget pairs an output path with the rows destined for it.
type outTarget struct {
	path string
	rows [][]string
}

// outTargets returns the main output plus the train/test variants when the run
// was split.
func outTargets(base, ext string, result *Result) []outTarget {
	targets := []outTarget{{base, result.Rows}}
	if result.Train != nil {
		stem := strings.TrimSuffix(base, ext)
		targets = append(targets,
			outTarget{stem + "_train" + ext, result.Train},
			outTarget{stem + "_test" + ext, result.Test})
	}
	return targets
}

// writeOutputs writes every requested output format and returns the paths.
func writeOutputs(cfg *Config, result *Result, opts Options) ([]string, error) {
	csvWanted, parquetWanted := false, false
	for _, format := range cfg.OutputFormats.Items() {
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "csv":
			csvWanted = true
		case "parquet":
			parquetWanted = true
		}
	}
	if cfg.OutputFormats.Len() == 0 {
		csvWanted = true
	}

	opts.progress(Progress{Phase: PhaseWrite, Done: 0, Total: 1})
	var files []string

	withHeader := func(rows [][]string) [][]string {
		return append([][]string{result.Headers}, rows...)
	}

	if csvWanted {
		for _, target := range outTargets(cfg.Outfile, ".csv", result) {
			if err := writeToCSV(target.path, withHeader(target.rows)); err != nil {
				return files, fmt.Errorf("failed to write CSV: %w", err)
			}
			opts.logf("wrote %s (%d rows)", target.path, len(target.rows))
			files = append(files, target.path)
		}
	}

	if parquetWanted {
		// A .csv outfile becomes .parquet so both formats can coexist.
		base := cfg.Outfile
		if strings.HasSuffix(base, ".csv") {
			base = strings.TrimSuffix(base, ".csv") + ".parquet"
		}
		for _, target := range outTargets(base, ".parquet", result) {
			written, err := writeToParquet(target.path, withHeader(target.rows), cfg)
			files = append(files, written...)
			if err != nil {
				return files, fmt.Errorf("failed to write Parquet: %w", err)
			}
			opts.logf("wrote %s (%d rows)", target.path, len(target.rows))
		}
	}

	opts.progress(Progress{Phase: PhaseWrite, Done: 1, Total: 1})
	return files, nil
}
