package engine

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/markcheno/go-quote"
)

// DateLayout is the format for start_date and end_date, and for the date
// column of daily and coarser bars.
const DateLayout = "2006-01-02"

// TimestampLayout is the date column format for intraday bars, where a bare
// date would give every bar in a day the same value. It sorts lexicographically
// and parses as a prefix superset of DateLayout.
const TimestampLayout = "2006-01-02 15:04:05"

// Severity distinguishes problems that block a run from ones that merely
// surprise the user.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// FieldError attributes a validation problem to a config field so a UI can
// render it next to the right input.
type FieldError struct {
	Field    string   `json:"field"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
	// Index identifies which entry of a list field is at fault, or -1.
	Index int `json:"index"`
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type problems []FieldError

func (p *problems) err(field, format string, args ...any) {
	*p = append(*p, FieldError{Field: field, Message: fmt.Sprintf(format, args...), Severity: SeverityError, Index: -1})
}

func (p *problems) errAt(field string, index int, format string, args ...any) {
	*p = append(*p, FieldError{Field: field, Message: fmt.Sprintf(format, args...), Severity: SeverityError, Index: index})
}

func (p *problems) warn(field, format string, args ...any) {
	*p = append(*p, FieldError{Field: field, Message: fmt.Sprintf(format, args...), Severity: SeverityWarning, Index: -1})
}

// identRe matches a legal column name.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// reservedColumnNames are bound by the evaluator for every ticker and so cannot
// be used as user column names.
var reservedColumnNames = []string{"d", "o", "h", "l", "c", "v", "result", "symbol", "date", "open", "high", "low", "close", "volume"}

// Validate checks cfg and returns every problem found. An empty result means
// the config is runnable. Warnings are included; use HasErrors to decide
// whether to proceed.
func Validate(cfg *Config) []FieldError {
	var p problems

	validateDates(cfg, &p)
	validateUniverse(cfg, &p)
	validateColumns(cfg, &p)
	validateOutput(cfg, &p)
	validateTransform(cfg, &p)
	validateParquet(cfg, &p)

	return p
}

// HasErrors reports whether any problem is blocking.
func HasErrors(errs []FieldError) bool {
	return slices.ContainsFunc(errs, func(e FieldError) bool { return e.Severity == SeverityError })
}

// FirstError returns the first blocking problem, or nil.
func FirstError(errs []FieldError) error {
	for _, e := range errs {
		if e.Severity == SeverityError {
			return e
		}
	}
	return nil
}

// parseDate validates a date string. go-quote's own parser discards its error
// and silently yields the zero time, and panics outright on strings longer than
// its layout, so nothing downstream will catch a bad date.
func parseDate(field, value string, p *problems) (time.Time, bool) {
	if value == "" {
		p.err(field, "date is required (YYYY-MM-DD)")
		return time.Time{}, false
	}
	t, err := time.Parse(DateLayout, value)
	if err != nil {
		p.err(field, "invalid date %q, expected YYYY-MM-DD", value)
		return time.Time{}, false
	}
	return t, true
}

func validateDates(cfg *Config, p *problems) {
	start, okStart := parseDate("start_date", cfg.StartDate, p)
	end, okEnd := parseDate("end_date", cfg.EndDate, p)
	if okStart && okEnd && end.Before(start) {
		p.err("end_date", "end date %s is before start date %s", cfg.EndDate, cfg.StartDate)
	}
}

func validateUniverse(cfg *Config, p *problems) {
	sources := Sources()
	switch {
	case cfg.Source == "":
		p.err("source", "source is required")
	case !slices.Contains(sources, cfg.Source):
		p.err("source", "invalid source %q, expected one of %s", cfg.Source, strings.Join(sources, ", "))
	default:
		validatePeriod(cfg, p)
	}

	// go-quote's provider registry does not describe credentials, so which
	// sources need a token is still knowledge go-scan holds itself.
	if (cfg.Source == "tiingo" || cfg.Source == "tiingo-crypto") && cfg.TiingoToken == "" {
		p.err("tiingo_token", "a Tiingo API token is required for source %q (set TIINGO_API_TOKEN or pass -tiingo-token)", cfg.Source)
	}

	if cfg.Market != "" {
		switch {
		case !ValidMarket(cfg.Market):
			p.err("market", "invalid market %q", cfg.Market)
		case quote.MarketRequiresToken(cfg.Market) && cfg.TiingoToken == "":
			p.err("market", "market %q requires a Tiingo API token", cfg.Market)
		}
	}

	if cfg.Market == "" && cfg.Tickers.Len() == 0 {
		p.err("tickers", "specify at least one ticker, or a market")
	}
	for i, t := range cfg.Tickers.Items() {
		if strings.TrimSpace(t) == "" {
			p.errAt("tickers", i, "empty ticker")
		}
	}
}

// validatePeriod checks the period spelling, then whether the chosen source
// actually serves it. Only called once the source is known to be valid.
func validatePeriod(cfg *Config, p *problems) {
	period, err := ParsePeriod(cfg.Period)
	if err != nil {
		p.err("period", "%s", err)
		return
	}
	provider, err := providerClient.Provider(cfg.Source)
	if err != nil {
		return // already reported as a source problem
	}
	if err := quote.CheckPeriod(provider, period); err != nil {
		p.err("period", "%s", err)
	}
}

func validateColumns(cfg *Config, p *problems) {
	seen := map[string]int{}
	for i, spec := range cfg.Columns.Items() {
		if strings.TrimSpace(spec) == "" {
			p.errAt("columns", i, "empty column definition")
			continue
		}
		name, expr, ok := strings.Cut(spec, "=")
		name = strings.TrimSpace(name)
		expr = strings.TrimSpace(expr)
		if !ok || name == "" || expr == "" {
			p.errAt("columns", i, "expected name=expression, got %q", spec)
			continue
		}
		if !identRe.MatchString(name) {
			p.errAt("columns", i, "%q is not a valid column name (letters, digits and underscore, not starting with a digit)", name)
		}
		if slices.Contains(reservedColumnNames, name) {
			p.errAt("columns", i, "%q is reserved and cannot be used as a column name", name)
		}
		if prev, dup := seen[name]; dup {
			p.errAt("columns", i, "column %q is already defined at position %d", name, prev+1)
		}
		seen[name] = i
	}

	if cfg.Filter != "" && cfg.Pivot {
		p.warn("filter", "the filter is applied per ticker before pivoting")
	}
}

func validateOutput(cfg *Config, p *problems) {
	if cfg.Outfile == "" {
		p.err("outfile", "an output file is required")
		return
	}

	csvWanted, parquetWanted := false, false
	for i, format := range cfg.OutputFormats.Items() {
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "csv":
			csvWanted = true
		case "parquet":
			parquetWanted = true
		default:
			p.errAt("output_formats", i, "unknown output format %q, expected csv or parquet", format)
		}
	}
	if cfg.OutputFormats.Len() == 0 {
		csvWanted = true // default
	}

	switch {
	case csvWanted && parquetWanted:
		// Either extension is fine; the parquet writer swaps .csv for .parquet.
	case csvWanted && !strings.HasSuffix(cfg.Outfile, ".csv"):
		p.err("outfile", "output file must end in .csv when only CSV output is requested")
	case parquetWanted && !csvWanted && !strings.HasSuffix(cfg.Outfile, ".parquet"):
		p.err("outfile", "output file must end in .parquet when only Parquet output is requested")
	}
}

func validateTransform(cfg *Config, p *problems) {
	if cfg.Truncate < 0 {
		p.err("truncate", "truncate must not be negative")
	}
	if cfg.SplitPct < 0 || cfg.SplitPct > 1 {
		p.err("split_pct", "split_pct must be between 0 and 1 (0 disables the train/test split)")
	}
	if cfg.TargetColumn != "" && !cfg.Pivot {
		p.warn("target_column", "target_column is only applied when pivot is enabled")
	}

	headers := ProjectedHeaders(cfg)
	for _, col := range splitCommaList(cfg.DropColumns) {
		if !containsFold(headers, col) {
			p.warn("drop_columns", "no column named %q will be produced", col)
		}
	}
}

func validateParquet(cfg *Config, p *problems) {
	wantsParquet := slices.ContainsFunc(cfg.OutputFormats.Items(), func(f string) bool {
		return strings.EqualFold(strings.TrimSpace(f), "parquet")
	})
	if !wantsParquet {
		return
	}

	if cfg.ParquetCompression != "" &&
		!slices.Contains(Compressions, strings.ToLower(cfg.ParquetCompression)) &&
		!strings.EqualFold(cfg.ParquetCompression, "uncompressed") {
		p.err("parquet_compression", "unknown codec %q, expected one of %s", cfg.ParquetCompression, strings.Join(Compressions, ", "))
	}
	if cfg.ParquetRowGroupSize < 0 {
		p.err("parquet_row_group_size", "row group size must not be negative")
	}

	partitionsByDate := slices.Contains(cfg.ParquetPartitionBy.Items(), "date")
	if partitionsByDate && cfg.ParquetPartitionDateFmt != "" &&
		!slices.Contains(PartitionDateFormats, cfg.ParquetPartitionDateFmt) {
		p.err("parquet_partition_date_format", "expected one of %s", strings.Join(PartitionDateFormats, ", "))
	}
	if !partitionsByDate && cfg.ParquetPartitionDateFmt != "" {
		p.warn("parquet_partition_date_format", "only applied when partitioning by date")
	}

	// Sort and partition columns silently no-op when they name a column that
	// will not exist, which is easy to do and hard to notice.
	headers := ProjectedHeaders(cfg)
	if !cfg.Pivot {
		for i, col := range cfg.ParquetPartitionBy.Items() {
			if !containsFold(headers, col) {
				p.errAt("parquet_partition_by", i, "no column named %q will be produced", col)
			}
		}
		for i, col := range cfg.ParquetSortBy.Items() {
			if !containsFold(headers, col) {
				p.errAt("parquet_sort_by", i, "no column named %q will be produced", col)
			}
		}
	}
}

// ProjectedHeaders returns the headers a run of cfg would produce, before
// pivoting. It is used both for validation and to seed the UI's column pickers.
func ProjectedHeaders(cfg *Config) []string {
	headers := append([]string{}, BaseHeaders...)
	for _, spec := range cfg.Columns.Items() {
		if name, _, ok := strings.Cut(spec, "="); ok {
			if name = strings.TrimSpace(name); name != "" {
				headers = append(headers, name)
			}
		}
	}
	for _, drop := range splitCommaList(cfg.DropColumns) {
		headers = slices.DeleteFunc(headers, func(h string) bool { return strings.EqualFold(h, drop) })
	}
	return headers
}

func splitCommaList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func containsFold(haystack []string, needle string) bool {
	return slices.ContainsFunc(haystack, func(h string) bool { return strings.EqualFold(h, needle) })
}

// ValidMarket reports whether go-quote can resolve the named market.
func ValidMarket(market string) bool {
	return quote.ValidMarket(market)
}
