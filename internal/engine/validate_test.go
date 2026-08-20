package engine

import (
	"slices"
	"testing"
)

func validConfig() Config {
	cfg := DefaultConfig()
	cfg.Source = "coinbase"
	cfg.Tickers = NewStringList("AAPL")
	cfg.Outfile = "out.csv"
	cfg.StartDate = "2024-01-01"
	cfg.EndDate = "2024-12-31"
	return cfg
}

func TestValidateAcceptsAGoodConfig(t *testing.T) {
	cfg := validConfig()
	if problems := Validate(&cfg); len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
}

// etf is listed as a valid market but go-quote has no URL for it; go-scan
// resolves it through NewEtfList instead, so it must not be rejected.
func TestValidateAcceptsEveryListedMarket(t *testing.T) {
	for _, market := range Markets() {
		cfg := validConfig()
		cfg.Market = market
		cfg.TiingoToken = "token" // the tiingo-* markets require one

		problems := Validate(&cfg)
		for _, p := range problems {
			if p.Field == "market" {
				t.Errorf("market %q rejected: %s", market, p.Message)
			}
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		field  string
	}{
		{"bad start date", func(c *Config) { c.StartDate = "01/02/2024" }, "start_date"},
		{"empty end date", func(c *Config) { c.EndDate = "" }, "end_date"},
		{"end before start", func(c *Config) { c.EndDate = "2023-01-01" }, "end_date"},
		{"over-long date does not panic", func(c *Config) { c.StartDate = "2024-01-01 00:00:00.000" }, "start_date"},
		{"unknown source", func(c *Config) { c.Source = "bloomberg" }, "source"},
		{"tiingo needs a token", func(c *Config) { c.Source = "tiingo"; c.TiingoToken = "" }, "tiingo_token"},
		{"unknown market", func(c *Config) { c.Market = "ftse" }, "market"},
		{"tiingo market needs a token", func(c *Config) { c.Market = "tiingo-btc"; c.TiingoToken = "" }, "market"},
		{"no universe", func(c *Config) { c.Tickers = NewStringList() }, "tickers"},
		{"malformed column", func(c *Config) { c.Columns = NewStringList("sma20") }, "columns"},
		{"reserved column name", func(c *Config) { c.Columns = NewStringList("close=sma(c,20)") }, "columns"},
		{"duplicate column name", func(c *Config) { c.Columns = NewStringList("a=sma(c,2)", "a=sma(c,3)") }, "columns"},
		{"invalid column identifier", func(c *Config) { c.Columns = NewStringList("2fast=sma(c,2)") }, "columns"},
		{"csv output needs a csv extension", func(c *Config) { c.Outfile = "out.dat" }, "outfile"},
		{"parquet output needs a parquet extension", func(c *Config) {
			c.OutputFormats = NewStringList("parquet")
		}, "outfile"},
		{"unknown output format", func(c *Config) { c.OutputFormats = NewStringList("csv", "avro") }, "output_formats"},
		{"negative truncate", func(c *Config) { c.Truncate = -1 }, "truncate"},
		{"split out of range", func(c *Config) { c.SplitPct = 1.5 }, "split_pct"},
		{"unknown compression", func(c *Config) {
			c.OutputFormats = NewStringList("parquet")
			c.Outfile = "out.parquet"
			c.ParquetCompression = "lz4"
		}, "parquet_compression"},
		{"partition by a column that will not exist", func(c *Config) {
			c.OutputFormats = NewStringList("parquet")
			c.Outfile = "out.parquet"
			c.ParquetPartitionBy = NewStringList("sector")
		}, "parquet_partition_by"},
		{"sort by a dropped column", func(c *Config) {
			c.OutputFormats = NewStringList("parquet")
			c.Outfile = "out.parquet"
			c.DropColumns = "volume"
			c.ParquetSortBy = NewStringList("volume")
		}, "parquet_sort_by"},
		{"bad partition date format", func(c *Config) {
			c.OutputFormats = NewStringList("parquet")
			c.Outfile = "out.parquet"
			c.ParquetPartitionBy = NewStringList("date")
			c.ParquetPartitionDateFmt = "quarter"
		}, "parquet_partition_date_format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)

			problems := Validate(&cfg)
			if !HasErrors(problems) {
				t.Fatalf("expected a blocking problem on %s, got %v", tt.field, problems)
			}
			if !slices.ContainsFunc(problems, func(p FieldError) bool {
				return p.Field == tt.field && p.Severity == SeverityError
			}) {
				t.Errorf("no error attributed to %s, got %v", tt.field, problems)
			}
		})
	}
}

func TestValidateWarnings(t *testing.T) {
	cfg := validConfig()
	cfg.TargetColumn = "AAPL_target" // meaningless without pivot
	cfg.DropColumns = "nosuchcolumn"

	problems := Validate(&cfg)
	if HasErrors(problems) {
		t.Fatalf("these should be warnings, not errors: %v", problems)
	}
	if len(problems) != 2 {
		t.Errorf("expected two warnings, got %v", problems)
	}
}

func TestProjectedHeaders(t *testing.T) {
	cfg := validConfig()
	cfg.Columns = NewStringList("sma20=sma(c,20)", "rsi2=rsi(c,2)")
	cfg.DropColumns = "open, high , low"

	want := []string{"symbol", "date", "close", "volume", "sma20", "rsi2"}
	if got := ProjectedHeaders(&cfg); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseColumnSpecsKeepsEqualsInExpressions(t *testing.T) {
	om, err := parseColumnSpecs([]string{"sig=c>=50", "  ", "b=sma(c,2)"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !slices.Equal(om.Keys(), []string{"sig", "b"}) {
		t.Fatalf("keys = %v", om.Keys())
	}
	expr, _ := om.Get("sig")
	if expr != "c>=50" {
		t.Errorf("expression = %q, want c>=50", expr)
	}
}
