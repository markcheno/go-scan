package engine

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/markcheno/go-quote"
)

// stubFetcher serves synthetic quotes so the pipeline can be tested without
// network access.
type stubFetcher struct {
	bars    int
	fail    map[string]string
	markets map[string][]string
}

func newStubFetcher(bars int) *stubFetcher {
	return &stubFetcher{bars: bars, fail: map[string]string{}, markets: map[string][]string{}}
}

func (s *stubFetcher) Fetch(ctx context.Context, cfg *Config, ticker string) (quote.Quote, error) {
	if msg, ok := s.fail[ticker]; ok {
		return quote.Quote{}, fmt.Errorf("%s", msg)
	}
	return synthQuote(ticker, s.bars), nil
}

func (s *stubFetcher) Market(ctx context.Context, market string) ([]string, error) {
	symbols, ok := s.markets[market]
	if !ok {
		return nil, fmt.Errorf("unknown market %s", market)
	}
	return symbols, nil
}

// synthQuote builds a deterministic rising series so expected values are easy
// to reason about.
func synthQuote(symbol string, bars int) quote.Quote {
	q := quote.Quote{Symbol: symbol}
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	base := float64(len(symbol) * 10)
	for i := range bars {
		close := base + float64(i)
		q.Date = append(q.Date, start.AddDate(0, 0, i))
		q.Open = append(q.Open, close-0.5)
		q.High = append(q.High, close+1)
		q.Low = append(q.Low, close-1)
		q.Close = append(q.Close, close)
		q.Volume = append(q.Volume, 1_000_000+float64(i))
	}
	return q
}

func baseConfig() *Config {
	cfg := DefaultConfig()
	cfg.Source = "coinbase" // no token required
	cfg.Outfile = "out.csv"
	cfg.StartDate = "2024-01-01"
	cfg.EndDate = "2024-12-31"
	return &cfg
}

func TestRunProducesExpectedTable(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BBBB")
	cfg.Columns = NewStringList("sma3=sma(c,3)", "up=gt(c,lag(c,1))")

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	wantHeaders := append(append([]string{}, BaseHeaders...), "sma3", "up")
	if !slices.Equal(result.Headers, wantHeaders) {
		t.Errorf("headers = %v, want %v", result.Headers, wantHeaders)
	}
	if len(result.Rows) != 20 {
		t.Errorf("rows = %d, want 20", len(result.Rows))
	}
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
	if len(result.Files) != 0 {
		t.Errorf("DryRun wrote files: %v", result.Files)
	}

	// Ticker order is preserved even though fetches run concurrently.
	if result.Rows[0][0] != "AAA" || result.Rows[10][0] != "BBBB" {
		t.Errorf("ticker order not preserved: %s then %s", result.Rows[0][0], result.Rows[10][0])
	}

	// sma3 of a series rising by 1 equals the middle value.
	col := slices.Index(result.Headers, "sma3")
	var got float64
	fmt.Sscanf(result.Rows[3][col], "%f", &got)
	if math.Abs(got-32) > 1e-6 { // AAA base 30, closes 30..39, sma3 at index 3 = 32
		t.Errorf("sma3 at row 3 = %v, want 32", got)
	}
}

func TestRunFilterExcludesTickersNumerically(t *testing.T) {
	cfg := baseConfig()
	// AAA closes end at 39, BBBB (4 chars) at 49.
	cfg.Tickers = NewStringList("AAA", "BBBB")
	cfg.Filter = "close > 45"

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(result.Verdicts) != 2 {
		t.Fatalf("verdicts = %d, want 2", len(result.Verdicts))
	}
	byTicker := map[string]bool{}
	for _, v := range result.Verdicts {
		byTicker[v.Ticker] = v.Passed
	}
	if byTicker["AAA"] {
		t.Error("AAA closes at 39 and should not pass close > 45")
	}
	if !byTicker["BBBB"] {
		t.Error("BBBB closes at 49 and should pass close > 45")
	}
	for _, row := range result.Rows {
		if row[0] != "BBBB" {
			t.Fatalf("filtered ticker %s leaked into the output", row[0])
		}
	}
}

func TestRunSplitExcludesFilteredTickers(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BBBB")
	cfg.Filter = "close > 45"
	cfg.SplitPct = 0.8

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Train)+len(result.Test) != len(result.Rows) {
		t.Errorf("train %d + test %d != rows %d", len(result.Train), len(result.Test), len(result.Rows))
	}
	for _, row := range append(append([][]string{}, result.Train...), result.Test...) {
		if row[0] != "BBBB" {
			t.Fatalf("filtered ticker %s leaked into the train/test split", row[0])
		}
	}
	if len(result.Train) != 8 {
		t.Errorf("train rows = %d, want 8", len(result.Train))
	}
}

// The split is per ticker, so every ticker contributes to both sets rather
// than whole tickers landing entirely in one of them.
func TestRunSplitsEachTickerSeparately(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BBBB")
	cfg.SplitPct = 0.8

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Train) != 16 || len(result.Test) != 4 {
		t.Fatalf("train %d / test %d, want 16 / 4", len(result.Train), len(result.Test))
	}
	for _, set := range map[string][][]string{"train": result.Train, "test": result.Test} {
		seen := map[string]bool{}
		for _, row := range set {
			seen[row[0]] = true
		}
		if !seen["AAA"] || !seen["BBBB"] {
			t.Errorf("both tickers should appear in each set, saw %v", seen)
		}
	}
	// Each ticker's test rows must come after its train rows.
	if result.Test[0][1] <= result.Train[7][1] {
		t.Errorf("test starts at %s but train ends at %s", result.Test[0][1], result.Train[7][1])
	}
}

func TestRunContinuesPastFetchFailures(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BAD", "CCC")

	fetcher := newStubFetcher(5)
	fetcher.fail["BAD"] = "delisted"

	result, err := Run(t.Context(), cfg, Options{Fetcher: fetcher, DryRun: true})
	if err != nil {
		t.Fatalf("run should not abort on a single bad ticker: %v", err)
	}
	if len(result.Errors) != 1 || result.Errors[0].Ticker != "BAD" {
		t.Fatalf("errors = %v, want one for BAD", result.Errors)
	}
	if len(result.Rows) != 10 {
		t.Errorf("rows = %d, want 10 from the two good tickers", len(result.Rows))
	}
}

func TestRunSurvivesEmptyAndOverTruncatedTickers(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")
	cfg.Truncate = 50

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("rows = %d, want 0", len(result.Rows))
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected one error explaining the empty result, got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Message, "truncate") {
		t.Errorf("error should mention truncate, got %q", result.Errors[0].Message)
	}
}

func TestRunBadColumnIsReportedNotFatal(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BBBB")
	cfg.Columns = NewStringList("bogus=nosuchfunc(c,2)")

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(5), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Errors) != 2 {
		t.Fatalf("expected an error per ticker, got %v", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Message, "bogus") {
		t.Errorf("error should name the column, got %q", result.Errors[0].Message)
	}
}

func TestRunMismatchedSeriesLengthIsAnError(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")
	cfg.Columns = NewStringList("bad=gt(c,series(1,3))")

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10), DryRun: true})
	if err != nil {
		t.Fatalf("a panic in a vector helper must not escape Run: %v", err)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "lengths differ") {
		t.Fatalf("errors = %v, want a length mismatch", result.Errors)
	}
}

func TestRunResolvesMarkets(t *testing.T) {
	cfg := baseConfig()
	cfg.Market = "nasdaq100"
	cfg.Tickers = NewStringList("AAA") // union'd with the market, deduplicated

	fetcher := newStubFetcher(3)
	fetcher.markets["nasdaq100"] = []string{"AAA", "BBBB"}

	result, err := Run(t.Context(), cfg, Options{Fetcher: fetcher, DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !slices.Equal(result.Tickers, []string{"AAA", "BBBB"}) {
		t.Errorf("tickers = %v, want [AAA BBBB]", result.Tickers)
	}
}

func TestRunMaxTickersAndMaxBars(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BBBB", "CCCCC")

	result, err := Run(t.Context(), cfg, Options{
		Fetcher: newStubFetcher(100), DryRun: true, MaxTickers: 2, MaxBars: 5,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// A sampled run still reports the size of the full universe, so a preview
	// of a 4000-symbol market does not look like a 3-symbol one.
	if result.Universe != 3 {
		t.Errorf("universe = %d, want 3", result.Universe)
	}
	if len(result.Tickers) != 2 {
		t.Errorf("processed %d tickers, want 2", len(result.Tickers))
	}
	if len(result.Rows) != 10 {
		t.Errorf("rows = %d, want 10 (2 tickers x 5 bars)", len(result.Rows))
	}
	// MaxBars keeps the most recent bars, so the filter still sees the last row.
	if result.Rows[4][1] != "2024-04-09" {
		t.Errorf("last AAA date = %s, want the most recent bar", result.Rows[4][1])
	}
}

func TestRunWritesCSVAndSplits(t *testing.T) {
	dir := t.TempDir()
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")
	cfg.Columns = NewStringList("sma3=sma(c,3)")
	cfg.Outfile = filepath.Join(dir, "nested", "out.csv")
	cfg.SplitPct = 0.8

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(10)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("files = %v, want main plus train and test", result.Files)
	}

	rows := readCSV(t, cfg.Outfile)
	if len(rows) != 11 {
		t.Errorf("main file has %d rows including the header, want 11", len(rows))
	}
	if !slices.Equal(rows[0], result.Headers) {
		t.Errorf("header row = %v, want %v", rows[0], result.Headers)
	}
	train := readCSV(t, filepath.Join(dir, "nested", "out_train.csv"))
	if len(train) != 9 {
		t.Errorf("train file has %d rows including the header, want 9", len(train))
	}
}

func TestRunDropColumnsRemovesTheRightOnes(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")
	cfg.Columns = NewStringList("sma3=sma(c,3)")
	cfg.DropColumns = "open,high,low,volume"

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(4), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []string{"symbol", "date", "close", "sma3"}
	if !slices.Equal(result.Headers, want) {
		t.Fatalf("headers = %v, want %v", result.Headers, want)
	}
	// The surviving values must still line up with their headers.
	if result.Rows[0][2] != "30.000000" {
		t.Errorf("close = %s, want 30.000000", result.Rows[0][2])
	}
}

func TestRunPivot(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA", "BBBB")
	cfg.Columns = NewStringList("target=roc(c,1)", "other_target=roc(c,2)")
	cfg.DropColumns = "open,high,low,volume"
	cfg.Pivot = true
	cfg.TargetColumn = "AAA_target"

	result, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(5), DryRun: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Headers[0] != "date" {
		t.Errorf("pivoted table should be indexed by date, got %v", result.Headers)
	}
	for _, h := range result.Headers {
		if strings.Contains(h, "target") && h != "AAA_target" {
			t.Errorf("target column %q should have been dropped", h)
		}
	}
	if !slices.Contains(result.Headers, "AAA_target") {
		t.Errorf("the kept target column is missing from %v", result.Headers)
	}
	if len(result.Rows) != 5 {
		t.Errorf("pivoted rows = %d, want one per date", len(result.Rows))
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")
	cfg.StartDate = "not-a-date"

	if _, err := Run(t.Context(), cfg, Options{Fetcher: newStubFetcher(5), DryRun: true}); err == nil {
		t.Fatal("expected an error for an invalid start date")
	}
}

func TestRunHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")

	if _, err := Run(ctx, cfg, Options{Fetcher: newStubFetcher(5), DryRun: true}); err == nil {
		t.Fatal("expected a cancellation error")
	}
}

// TestConcurrentEvaluationIsIsolated is the regression guard for the shared
// interpreter environment: concurrent evaluations must not see each other's
// bindings.
func TestConcurrentEvaluationIsIsolated(t *testing.T) {
	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)

	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			symbol := fmt.Sprintf("T%d", i)
			q := synthQuote(symbol, 40)
			ev := NewEvaluator()
			if err := ev.BindQuote(q); err != nil {
				errs[i] = err
				return
			}
			name := fmt.Sprintf("col%d", i)
			values, err := ev.Column(t.Context(), name, "sma(c,3)")
			if err != nil {
				errs[i] = err
				return
			}
			// Each worker's series starts at a value unique to its symbol.
			want := q.Close[2] - 1
			if math.Abs(values[2]-want) > 1e-9 {
				errs[i] = fmt.Errorf("worker %d saw %v, want %v", i, values[2], want)
				return
			}
			// A sibling's column must not be visible here.
			other := fmt.Sprintf("col%d", (i+1)%workers)
			if _, err := ev.Column(t.Context(), "probe", other); err == nil {
				errs[i] = fmt.Errorf("worker %d could see %s from another evaluator", i, other)
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestEvaluatorDoesNotLeakAcrossInstances(t *testing.T) {
	q := synthQuote("AAA", 10)

	first := NewEvaluator()
	if err := first.BindQuote(q); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Column(t.Context(), "sma20", "sma(c,3)"); err != nil {
		t.Fatal(err)
	}

	second := NewEvaluator()
	if err := second.BindQuote(q); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Column(t.Context(), "derived", "gt(sma20,c)"); err == nil {
		t.Error("sma20 from an earlier evaluator is still visible")
	}
}

func TestEvaluatorSeesEarlierColumns(t *testing.T) {
	q := synthQuote("AAA", 10)
	ev := NewEvaluator()
	if err := ev.BindQuote(q); err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Column(t.Context(), "sma3", "sma(c,3)"); err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Column(t.Context(), "above", "gt(c,sma3)"); err != nil {
		t.Fatalf("a later column must be able to reference an earlier one: %v", err)
	}
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}
