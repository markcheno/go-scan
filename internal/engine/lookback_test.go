package engine

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/markcheno/go-quote"
)

// lookbackFor runs the derivation over a set of column specs.
func lookbackFor(t *testing.T, columns ...string) (int, []string) {
	t.Helper()
	cfg := Config{Lookback: LookbackAuto, Columns: NewStringList(columns...)}
	bars, unresolved, err := LookbackBars(&cfg)
	if err != nil {
		t.Fatalf("LookbackBars(%v): %v", columns, err)
	}
	return bars, unresolved
}

func TestLookbackDerivesWindowSizes(t *testing.T) {
	tests := []struct {
		name    string
		columns []string
		want    int
	}{
		{"single window", []string{"sma200=sma(c,200)"}, 200},
		{"largest of several columns", []string{"a=sma(c,20)", "b=sma(c,200)", "z=rsi(c,2)"}, 200},
		{"no windows at all", []string{"x=add(h,l)"}, 0},
		{"raw series costs nothing", []string{"x=c"}, 0},

		// A window applied to an already-warming series cannot start counting
		// until that series is valid, so the two add.
		{"nested windows add", []string{"deep=sma(rsi(c,2),200)"}, 202},
		{"three deep", []string{"deep=sma(sma(sma(c,5),10),20)"}, 35},

		// Later columns may reference earlier ones by name, so the cost of the
		// reference carries into the referring column.
		{"column reference carries", []string{"sma200=sma(c,200)", "above=gt(c,sma200)"}, 200},
		{"window over a referenced column", []string{"base=sma(c,50)", "smooth=sma(base,10)"}, 60},

		// Several period arguments are summed. macd's true TA-Lib lookback is
		// 33; over-estimating is the safe direction.
		{"multiple periods sum", []string{"m=macd(c,12,26,9)"}, 47},
		{"hlc plus period", []string{"a=atr(h,l,c,14)"}, 14},

		// Non-period scalars must not be counted.
		{"nbdev is not a period", []string{"s=stddev(c,20,2)"}, 20},
		{"matype is not a period", []string{"b=bbands(c,20,2,2,SMA)"}, 20},

		// series(value,n) fabricates a constant series and needs no history.
		{"series length is not a window", []string{"k=series(1,100)"}, 0},

		// A fixed warm-up with no period argument to derive it from.
		{"hilbert transform", []string{"h=htsine(c)"}, 63},
		{"mama", []string{"m=mama(c,0.5,0.05)"}, 32},
		{"fixed warm-up composes", []string{"x=sma(htsine(c),10)"}, 73},

		// Arithmetic takes the larger operand.
		{"operators take the max", []string{"x=sma(c,200)-sma(c,50)"}, 200},
		{"parens and unary", []string{"x=-(sma(c,30))"}, 30},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, unresolved := lookbackFor(t, tc.columns...)
			if got != tc.want {
				t.Errorf("lookback = %d, want %d", got, tc.want)
			}
			if len(unresolved) != 0 {
				t.Errorf("unexpected unresolved: %v", unresolved)
			}
		})
	}
}

// A negative shift moves values backwards in time, consuming the tail rather
// than needing leading history.
func TestLookbackIgnoresForwardShift(t *testing.T) {
	if got, _ := lookbackFor(t, "target=shift(roc(c,5),-5)"); got != 5 {
		t.Errorf("forward shift lookback = %d, want 5 (roc only)", got)
	}
	if got, _ := lookbackFor(t, "lagged=shift(roc(c,5),5)"); got != 10 {
		t.Errorf("backward shift lookback = %d, want 10 (roc plus the lag)", got)
	}
	if got, _ := lookbackFor(t, "lagged=lag(c,3)"); got != 3 {
		t.Errorf("lag lookback = %d, want 3", got)
	}
}

func TestLookbackReportsWhatItCannotResolve(t *testing.T) {
	tests := []struct {
		name    string
		column  string
		wantSub string
	}{
		{"unknown function", "x=nosuchfunc(c,20)", "unknown function nosuchfunc"},
		{"unknown identifier", "x=sma(nosuchvar,20)", "unknown identifier nosuchvar"},
		{"non-literal period", "x=sma(c,someVar)", "non-literal period in sma"},
		{"unparseable", "x=sma(c,", "cannot parse"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, unresolved := lookbackFor(t, tc.column)
			if len(unresolved) == 0 {
				t.Fatalf("expected %q to be reported as unresolved", tc.column)
			}
			joined := strings.Join(unresolved, "; ")
			if !strings.Contains(joined, tc.wantSub) {
				t.Errorf("unresolved = %q, want it to mention %q", joined, tc.wantSub)
			}
			if !strings.Contains(joined, tc.column) {
				t.Errorf("unresolved = %q, want it to name the column spec", joined)
			}
		})
	}
}

// An unresolvable inner argument must not discard the parts that did resolve.
func TestLookbackKeepsResolvableParts(t *testing.T) {
	got, unresolved := lookbackFor(t, "x=nosuchfunc(sma(c,100))")
	if got != 100 {
		t.Errorf("lookback = %d, want 100 from the recognizable inner call", got)
	}
	if len(unresolved) == 0 {
		t.Error("expected the unknown outer function to be reported")
	}
}

func TestLookbackModes(t *testing.T) {
	columns := NewStringList("sma200=sma(c,200)")

	tests := []struct {
		mode string
		want int
	}{
		{"", 0},
		{"off", 0},
		{"OFF", 0},
		{"0", 0},
		{"auto", 200},
		{"AUTO", 200},
		{"50", 50},
		{"999999", maxLookbackBars},
	}

	for _, tc := range tests {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			cfg := Config{Lookback: tc.mode, Columns: columns}
			got, _, err := LookbackBars(&cfg)
			if err != nil {
				t.Fatalf("LookbackBars: %v", err)
			}
			if got != tc.want {
				t.Errorf("lookback = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestLookbackRejectsBadModes(t *testing.T) {
	for _, mode := range []string{"sometimes", "-5", "1.5"} {
		cfg := Config{Lookback: mode, Columns: NewStringList("x=sma(c,20)")}
		if _, _, err := LookbackBars(&cfg); err == nil {
			t.Errorf("LookbackBars(%q) succeeded, want an error", mode)
		}
	}
}

func TestLookbackIsCapped(t *testing.T) {
	got, _ := lookbackFor(t, "x=sma(c,999999)")
	if got != maxLookbackBars {
		t.Errorf("lookback = %d, want it capped at %d", got, maxLookbackBars)
	}
}

// The classification table drives the whole derivation, and a missing entry
// fails silently rather than loudly, so assert it covers the catalog.
func TestArgClassesCoverCatalog(t *testing.T) {
	for _, f := range Catalog() {
		for _, arg := range f.Args {
			if _, ok := argClasses[arg]; !ok {
				t.Errorf("%s: argument %q is not classified in argClasses", f.Signature(), arg)
			}
		}
	}
}

func TestWidenStart(t *testing.T) {
	tests := []struct {
		name   string
		period string
		bars   int
		want   string
	}{
		// 200 sessions is 290 calendar days at 365.25/252, plus the cushion.
		{"daily", "d", 200, "2023-03-04"},
		{"daily zero bars is untouched", "d", 0, "2024-01-01"},
		{"weekly", "w", 10, "2023-10-14"},
		{"monthly", "m", 12, "2022-12-10"},
		// 100 hourly bars over 6.5-hour sessions is 16 days, plus the cushion.
		{"hourly", "1h", 100, "2023-12-09"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := widenStart("2024-01-01", tc.bars, tc.period)
			if err != nil {
				t.Fatalf("widenStart: %v", err)
			}
			if got != tc.want {
				t.Errorf("widenStart(2024-01-01, %d, %q) = %s, want %s", tc.bars, tc.period, got, tc.want)
			}
		})
	}
}

// approxSessions counts the trading days in a span: weekdays, less the roughly
// nine days a year US markets close on top of weekends.
func approxSessions(from, to time.Time) int {
	weekdays := 0
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			weekdays++
		}
	}
	years := to.Sub(from).Hours() / 24 / 365.25
	return weekdays - int(math.Ceil(years*9))
}

// The widened window has to actually contain the bars it promises. Deriving it
// from weekends alone gives 7/5, which is short by the market holidays: 200 bars
// then reaches back far enough for only 198 sessions and an sma200 still opens
// with a zero. That is the defect this guards, and the exact-date cases above
// would not have caught it.
func TestWidenStartCoversEnoughTradingDays(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, bars := range []int{20, 200, 1000, maxLookbackBars} {
		got, err := widenStart(start.Format(DateLayout), bars, "d")
		if err != nil {
			t.Fatalf("widenStart(%d bars): %v", bars, err)
		}
		from, err := time.Parse(DateLayout, got)
		if err != nil {
			t.Fatalf("parsing %q: %v", got, err)
		}
		if sessions := approxSessions(from, start); sessions < bars {
			t.Errorf("widening for %d bars reaches back to %s, which holds only about %d sessions",
				bars, got, sessions)
		}
	}
}

// Every period the providers serve must produce a sane widening, so a new
// period upstream cannot silently fall through to a wrong default.
func TestWidenStartCoversEveryPeriod(t *testing.T) {
	for _, source := range Sources() {
		for _, period := range Periods(source) {
			got, err := widenStart("2024-06-01", 100, period)
			if err != nil {
				t.Fatalf("widenStart(period=%q): %v", period, err)
			}
			if got >= "2024-06-01" {
				t.Errorf("widenStart(period=%q) = %s, want an earlier date", period, got)
			}
		}
	}
}

func TestLookbackDaysGrowsWithBarLength(t *testing.T) {
	hourly := lookbackDays(100, quote.Min60)
	daily := lookbackDays(100, quote.Daily)
	weekly := lookbackDays(100, quote.Weekly)
	monthly := lookbackDays(100, quote.Monthly)

	if !(hourly < daily && daily < weekly && weekly < monthly) {
		t.Errorf("expected hourly < daily < weekly < monthly, got %d, %d, %d, %d",
			hourly, daily, weekly, monthly)
	}
}
