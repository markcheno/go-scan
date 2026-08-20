package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/markcheno/go-quote"
)

// dateStubFetcher serves one synthetic bar per calendar day across whatever
// range it is asked for, so widening the window actually yields more bars the
// way a real provider would. It records the start date it saw, which is how the
// tests observe that the fetch was widened.
type dateStubFetcher struct {
	mu       sync.Mutex
	seenFrom map[string]string
	// history bounds how far back data exists, for the short-history case.
	history string
}

func newDateStubFetcher() *dateStubFetcher {
	return &dateStubFetcher{seenFrom: map[string]string{}}
}

func (s *dateStubFetcher) Fetch(_ context.Context, cfg *Config, ticker string) (quote.Quote, error) {
	from, err := time.Parse(DateLayout, cfg.StartDate)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("start_date: %w", err)
	}
	to, err := time.Parse(DateLayout, cfg.EndDate)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("end_date: %w", err)
	}

	s.mu.Lock()
	s.seenFrom[ticker] = cfg.StartDate
	s.mu.Unlock()

	if s.history != "" {
		earliest, err := time.Parse(DateLayout, s.history)
		if err != nil {
			return quote.Quote{}, err
		}
		if from.Before(earliest) {
			from = earliest
		}
	}

	// Price is a function of the date, not of the position in the slice, so a
	// given day looks the same however wide the requested window was.
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	q := quote.Quote{Symbol: ticker}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		close := 100 + d.Sub(epoch).Hours()/24
		q.Date = append(q.Date, d)
		q.Open = append(q.Open, close-0.5)
		q.High = append(q.High, close+1)
		q.Low = append(q.Low, close-1)
		q.Close = append(q.Close, close)
		q.Volume = append(q.Volume, 1_000_000)
	}
	return q, nil
}

// lookbackConfig pins the lookback explicitly rather than inheriting the
// default, so these tests keep testing what they say they test if the default
// changes again.
func lookbackConfig(mode string) *Config {
	cfg := baseConfig()
	cfg.Tickers = NewStringList("AAA")
	cfg.Columns = NewStringList("sma20=sma(c,20)")
	cfg.EndDate = "2024-03-31"
	cfg.Lookback = mode
	return cfg
}

// cell finds a column's value in a row by header name.
func cell(t *testing.T, result *Result, row int, column string) string {
	t.Helper()
	for i, name := range result.Headers {
		if name == column {
			return result.Rows[row][i]
		}
	}
	t.Fatalf("no column %q in %v", column, result.Headers)
	return ""
}

func TestRunWidensTheFetchWindowForWarmup(t *testing.T) {
	cfg := lookbackConfig(LookbackAuto)

	fetcher := newDateStubFetcher()
	result, err := Run(t.Context(), cfg, Options{Fetcher: fetcher, DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The fetch reached back past the requested start.
	if got := fetcher.seenFrom["AAA"]; got >= cfg.StartDate {
		t.Errorf("fetched from %s, want a date before %s", got, cfg.StartDate)
	}
	if result.FetchFrom != fetcher.seenFrom["AAA"] {
		t.Errorf("Result.FetchFrom = %q, want %q", result.FetchFrom, fetcher.seenFrom["AAA"])
	}
	if result.Lookback != 20 {
		t.Errorf("Result.Lookback = %d, want 20", result.Lookback)
	}

	// But the output still begins where the user asked.
	if got := cell(t, result, 0, "date"); got != cfg.StartDate {
		t.Errorf("first row date = %s, want %s", got, cfg.StartDate)
	}

	// And the indicator is already warm there, which is the whole point.
	first, err := strconv.ParseFloat(cell(t, result, 0, "sma20"), 64)
	if err != nil {
		t.Fatalf("parsing sma20: %v", err)
	}
	if first == 0 {
		t.Error("sma20 is still zero on the first row; the warm-up bars did not take effect")
	}
}

// Without a lookback the leading rows are the zero-filled warm-up, which is the
// behavior the feature exists to fix and must be preserved when it is off.
func TestRunWithoutLookbackKeepsWarmupZeros(t *testing.T) {
	cfg := lookbackConfig(LookbackOff)

	fetcher := newDateStubFetcher()
	result, err := Run(t.Context(), cfg, Options{Fetcher: fetcher, DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := fetcher.seenFrom["AAA"]; got != cfg.StartDate {
		t.Errorf("fetched from %s, want exactly %s when lookback is off", got, cfg.StartDate)
	}
	if result.Lookback != 0 || result.FetchFrom != "" {
		t.Errorf("Result.Lookback/FetchFrom = %d/%q, want 0/\"\"", result.Lookback, result.FetchFrom)
	}
	if got := cell(t, result, 0, "sma20"); got != "0.000000" {
		t.Errorf("sma20 on the first row = %s, want 0.000000", got)
	}
}

// The lookback must buy warm indicators without costing rows: the requested
// range comes back whole either way.
func TestRunLookbackPreservesTheRequestedRange(t *testing.T) {
	off := lookbackConfig(LookbackOff)
	offResult, err := Run(t.Context(), off, Options{Fetcher: newDateStubFetcher(), DryRun: true})
	if err != nil {
		t.Fatalf("Run without lookback: %v", err)
	}

	on := lookbackConfig(LookbackAuto)
	onResult, err := Run(t.Context(), on, Options{Fetcher: newDateStubFetcher(), DryRun: true})
	if err != nil {
		t.Fatalf("Run with lookback: %v", err)
	}

	if len(offResult.Rows) != len(onResult.Rows) {
		t.Errorf("row count changed with lookback: %d without, %d with",
			len(offResult.Rows), len(onResult.Rows))
	}
	for _, column := range []string{"date", "close"} {
		for _, row := range []int{0, len(onResult.Rows) - 1} {
			if got, want := cell(t, onResult, row, column), cell(t, offResult, row, column); got != want {
				t.Errorf("row %d %s = %s with lookback, %s without", row, column, got, want)
			}
		}
	}
}

// truncate keeps meaning "drop N bars of output", counted from the requested
// start rather than from the warm-up bars.
func TestRunLookbackComposesWithTruncate(t *testing.T) {
	cfg := lookbackConfig(LookbackAuto)
	cfg.Truncate = 5

	result, err := Run(t.Context(), cfg, Options{Fetcher: newDateStubFetcher(), DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := cell(t, result, 0, "date"), "2024-01-06"; got != want {
		t.Errorf("first row date = %s, want %s (start plus a truncate of 5)", got, want)
	}
}

// A provider with less history than asked for is not an error; the run goes
// ahead with whatever warm-up it managed to get.
func TestRunLookbackToleratesShortHistory(t *testing.T) {
	cfg := lookbackConfig(LookbackAuto)

	fetcher := newDateStubFetcher()
	fetcher.history = "2023-12-28" // only a handful of bars before the start

	var logs []string
	result, err := Run(t.Context(), cfg, Options{
		Fetcher: fetcher, DryRun: true,
		Log: func(msg string) { logs = append(logs, msg) },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := cell(t, result, 0, "date"); got != cfg.StartDate {
		t.Errorf("first row date = %s, want %s", got, cfg.StartDate)
	}

	var warned bool
	for _, msg := range logs {
		if strings.Contains(msg, "lookback bars available") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a short-history warning, got logs: %v", logs)
	}
}

// MaxBars keeps only the most recent bars, so a preview of a config starting
// years back still opens mid-history. TotalRows is what lets the UI say so
// rather than looking like missing data.
func TestRunReportsRowsHeldBackByMaxBars(t *testing.T) {
	cfg := lookbackConfig(LookbackAuto)
	cfg.EndDate = "2024-12-31" // 366 daily bars from the stub

	capped, err := Run(t.Context(), cfg, Options{
		Fetcher: newDateStubFetcher(), DryRun: true, MaxBars: 50,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(capped.Rows) != 50 {
		t.Fatalf("got %d rows, want the 50 MaxBars allows", len(capped.Rows))
	}

	full, err := Run(t.Context(), cfg, Options{Fetcher: newDateStubFetcher(), DryRun: true})
	if err != nil {
		t.Fatalf("Run uncapped: %v", err)
	}
	if capped.TotalRows != len(full.Rows) {
		t.Errorf("TotalRows = %d, want %d, the row count without MaxBars",
			capped.TotalRows, len(full.Rows))
	}

	// With no cap the two must agree, so the UI can compare them directly.
	if full.TotalRows != len(full.Rows) {
		t.Errorf("uncapped TotalRows = %d, want %d", full.TotalRows, len(full.Rows))
	}
}

// A ticker the filter excluded contributes no rows, so it must not inflate the
// count either.
func TestTotalRowsCountsOnlyKeptTickers(t *testing.T) {
	cfg := lookbackConfig(LookbackAuto)
	cfg.Tickers = NewStringList("AAA", "BBB")
	cfg.Filter = "symbol == \"AAA\""

	result, err := Run(t.Context(), cfg, Options{Fetcher: newDateStubFetcher(), DryRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.TotalRows != len(result.Rows) {
		t.Errorf("TotalRows = %d, want %d; an excluded ticker was counted",
			result.TotalRows, len(result.Rows))
	}
}

// Widening the window changes what is fetched, so it must change the cache key
// too; otherwise a warmed run would serve a cold cached range, or vice versa.
func TestLookbackChangesTheCacheKey(t *testing.T) {
	cold := cacheKey("quote", "coinbase", "AAA", "2024-01-01", "2024-03-31", "d")
	warm := cacheKey("quote", "coinbase", "AAA", "2023-11-27", "2024-03-31", "d")
	if cold == warm {
		t.Error("the widened range produced the same cache key as the narrow one")
	}
}
