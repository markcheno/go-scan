package engine

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/markcheno/go-quote"
	"github.com/mattn/anko/ast"
	"github.com/mattn/anko/parser"
)

// The non-numeric values the lookback config field accepts. Anything else must
// parse as a non-negative bar count.
const (
	LookbackAuto = "auto"
	LookbackOff  = "off"
)

// maxLookbackBars caps a derived lookback so one runaway expression cannot widen
// the fetch window by decades.
const maxLookbackBars = 5000

// argClass says how one argument of a catalog function affects warm-up.
type argClass int

const (
	// argUnclassified is the zero value, so an argument name missing from
	// argClasses is walked as a series rather than silently ignored.
	argUnclassified argClass = iota
	// argIgnore is a scalar that needs no history: a multiplier, a limit, a
	// matype constant.
	argIgnore
	// argSeriesArg is a series to recurse into.
	argSeriesArg
	// argPeriodArg is a window length that adds to warm-up.
	argPeriodArg
)

// argClasses maps every argument name used in the catalog to its effect on
// warm-up. TestArgClassesCoverCatalog asserts this covers the whole catalog, so
// a new function with a new argument name fails until it is classified here.
var argClasses = map[string]argClass{
	// Series inputs.
	"series":  argSeriesArg,
	"series1": argSeriesArg,
	"series2": argSeriesArg,
	"returns": argSeriesArg,
	"o":       argSeriesArg,
	"h":       argSeriesArg,
	"l":       argSeriesArg,
	"c":       argSeriesArg,
	"v":       argSeriesArg,

	// Window lengths. These are summed, which over-estimates for functions
	// taking several — macd(c,12,26,9) yields 47 against a true 33 — but the
	// extra bars are discarded once the columns are computed, so erring long
	// costs nothing and erring short leaves the column warming.
	"period":    argPeriodArg,
	"period1":   argPeriodArg,
	"period2":   argPeriodArg,
	"period3":   argPeriodArg,
	"minperiod": argPeriodArg,
	"maxperiod": argPeriodArg,
	"fast":      argPeriodArg,
	"slow":      argPeriodArg,
	"signal":    argPeriodArg,
	"fastk":     argPeriodArg,
	"fastd":     argPeriodArg,
	"slowk":     argPeriodArg,
	"slowd":     argPeriodArg,
	"window":    argPeriodArg,

	// Scalars. Note "n" is the length of series(value,n), which fabricates a
	// constant series and so needs no history at all.
	"n":               argIgnore,
	"value":           argIgnore,
	"startvalue":      argIgnore,
	"maximum":         argIgnore,
	"offsetonreverse": argIgnore,
	"matype":          argIgnore,
	"fastmatype":      argIgnore,
	"fastdmatype":     argIgnore,
	"slowkmatype":     argIgnore,
	"slowdmatype":     argIgnore,
	"nbdev":           argIgnore,
	"nbdevup":         argIgnore,
	"nbdevdn":         argIgnore,
	"vfactor":         argIgnore,
	"fastlimit":       argIgnore,
	"slowlimit":       argIgnore,
	"acceleration":    argIgnore,
	"accellong":       argIgnore,
	"accelshort":      argIgnore,
	"accelinitlong":   argIgnore,
	"accelinitshort":  argIgnore,
	"accelmaxlong":    argIgnore,
	"accelmaxshort":   argIgnore,
}

// fixedWarmup carries the warm-up of functions that have one but expose no
// period argument to derive it from, so they would otherwise estimate zero.
// The values are TA-Lib's unstable-period defaults; go-talib exposes no
// Lookback helpers to read them from.
var fixedWarmup = map[string]int{
	"htdcperiod":  63,
	"htdcphase":   63,
	"htphasor":    63,
	"htsine":      63,
	"httrendmode": 63,
	"httrendline": 63,
	"mama":        32,
}

// quoteVars are the series BindQuote makes available to every expression.
var quoteVars = map[string]bool{
	"d": true, "o": true, "h": true, "l": true, "c": true, "v": true,
}

// LookbackBars resolves the lookback config field to a bar count: zero when it
// is off, the configured count when one is given, and the derived requirement
// for "auto". Expressions that cannot be resolved statically are named in
// unresolved and contribute nothing to the total.
func LookbackBars(cfg *Config) (bars int, unresolved []string, err error) {
	switch mode := strings.ToLower(strings.TrimSpace(cfg.Lookback)); mode {
	case "", LookbackOff, "0", "false", "no":
		return 0, nil, nil
	case LookbackAuto:
		return deriveLookback(cfg)
	default:
		n, convErr := strconv.Atoi(mode)
		if convErr != nil || n < 0 {
			return 0, nil, fmt.Errorf("expected %q, %q or a non-negative number of bars, got %q",
				LookbackAuto, LookbackOff, cfg.Lookback)
		}
		return min(n, maxLookbackBars), nil, nil
	}
}

// deriveLookback walks the column expressions and returns the largest warm-up
// any of them needs.
func deriveLookback(cfg *Config) (int, []string, error) {
	columnMap, err := parseColumnSpecs(cfg.Columns.Items())
	if err != nil {
		return 0, nil, err
	}

	s := &lookbackScope{columns: make(map[string]int)}
	bars := 0
	for _, name := range columnMap.Keys() {
		raw, _ := columnMap.Get(name)
		expr, _ := raw.(string)

		s.current = name + "=" + expr
		cost, err := s.exprCost(expr)
		if err != nil {
			s.note(err.Error())
			continue
		}
		// Later columns may reference earlier ones by name, so each column's
		// cost is available to the ones that follow it.
		s.columns[name] = cost
		bars = max(bars, cost)
	}
	return min(bars, maxLookbackBars), s.unresolved, nil
}

// lookbackScope carries the columns declared so far and the expressions that
// could not be resolved.
type lookbackScope struct {
	columns    map[string]int
	unresolved []string
	current    string
}

// note records the column being walked as unresolvable, once.
func (s *lookbackScope) note(reason string) {
	entry := s.current
	if reason != "" {
		entry += " (" + reason + ")"
	}
	for _, existing := range s.unresolved {
		if existing == entry {
			return
		}
	}
	s.unresolved = append(s.unresolved, entry)
}

func (s *lookbackScope) exprCost(expr string) (int, error) {
	stmt, err := parser.ParseSrc(expr)
	if err != nil {
		return 0, fmt.Errorf("cannot parse: %w", err)
	}
	node, ok := soleExpr(stmt)
	if !ok {
		return 0, fmt.Errorf("not a single expression")
	}
	return s.cost(node), nil
}

// soleExpr unwraps the single expression statement ParseSrc produces for an
// expression source.
func soleExpr(stmt ast.Stmt) (ast.Expr, bool) {
	if stmts, ok := stmt.(*ast.StmtsStmt); ok {
		if len(stmts.Stmts) != 1 {
			return nil, false
		}
		stmt = stmts.Stmts[0]
	}
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	return exprStmt.Expr, true
}

// cost reports how many bars of history an expression needs before it produces
// a valid value.
func (s *lookbackScope) cost(node ast.Expr) int {
	switch e := node.(type) {
	case *ast.CallExpr:
		return s.callCost(e)

	case *ast.IdentExpr:
		if quoteVars[e.Lit] {
			return 0
		}
		// Later columns may reference earlier ones by name.
		if cost, declared := s.columns[e.Lit]; declared {
			return cost
		}
		// Catalog entries appear as bare identifiers when they are matype
		// constants such as SMA.
		if _, ok := Lookup(e.Lit); ok {
			return 0
		}
		s.note("unknown identifier " + e.Lit)
		return 0

	case *ast.LiteralExpr:
		return 0

	case *ast.ParenExpr:
		return s.cost(e.SubExpr)

	case *ast.UnaryExpr:
		return s.cost(e.Expr)

	case *ast.OpExpr:
		return s.opCost(e.Op)

	default:
		return 0
	}
}

// opCost takes the larger requirement of the two operands. Every anko operator
// node carries an LHS and an RHS.
func (s *lookbackScope) opCost(op ast.Operator) int {
	var lhs, rhs ast.Expr
	switch o := op.(type) {
	case *ast.BinaryOperator:
		lhs, rhs = o.LHS, o.RHS
	case *ast.ComparisonOperator:
		lhs, rhs = o.LHS, o.RHS
	case *ast.AddOperator:
		lhs, rhs = o.LHS, o.RHS
	case *ast.MultiplyOperator:
		lhs, rhs = o.LHS, o.RHS
	default:
		return 0
	}
	return max(s.cost(lhs), s.cost(rhs))
}

// callCost is the function's own warm-up plus the deepest warm-up among the
// series passed to it, because a window applied to an already-warming series
// starts counting only once that series is valid.
func (s *lookbackScope) callCost(e *ast.CallExpr) int {
	def, known := Lookup(e.Name)
	if !known {
		s.note("unknown function " + e.Name)
		// Walk the arguments anyway so a recognizable inner call still counts.
		child := 0
		for _, arg := range e.SubExprs {
			child = max(child, s.cost(arg))
		}
		return child
	}

	// shift and lag take a signed period. A negative one shifts forward and
	// consumes the tail, needing no leading history.
	if e.Name == "shift" || e.Name == "lag" {
		child, own := 0, 0
		if len(e.SubExprs) > 0 {
			child = s.cost(e.SubExprs[0])
		}
		if len(e.SubExprs) > 1 {
			if n, ok := intLiteral(e.SubExprs[1]); ok {
				own = max(0, n)
			} else {
				s.note("non-literal period in " + e.Name)
			}
		}
		return child + own
	}

	own, child := 0, 0
	for i, arg := range e.SubExprs {
		name := ""
		if i < len(def.Args) {
			name = def.Args[i]
		}
		switch argClasses[name] {
		case argPeriodArg:
			n, ok := intLiteral(arg)
			if !ok {
				s.note("non-literal " + name + " in " + e.Name)
				continue
			}
			own += n
		case argIgnore:
			// A scalar; contributes nothing.
		default:
			child = max(child, s.cost(arg))
		}
	}
	if fixed, ok := fixedWarmup[e.Name]; ok {
		own = max(own, fixed)
	}
	return own + child
}

// intLiteral reads an integer constant, including one written with a unary
// minus. It reports false for anything that is not a compile-time constant.
func intLiteral(node ast.Expr) (int, bool) {
	switch e := node.(type) {
	case *ast.ParenExpr:
		return intLiteral(e.SubExpr)
	case *ast.UnaryExpr:
		n, ok := intLiteral(e.Expr)
		if !ok {
			return 0, false
		}
		if e.Operator == "-" {
			return -n, true
		}
		return n, true
	case *ast.LiteralExpr:
		return literalInt(e.Literal)
	}
	return 0, false
}

func literalInt(v reflect.Value) (int, bool) {
	if !v.IsValid() {
		return 0, false
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if f != math.Trunc(f) {
			return 0, false
		}
		return int(f), true
	}
	return 0, false
}

// barDuration is the wall-clock length of one bar, for the periods that have a
// fixed one. go-quote spells its period constants inconsistently (some as a
// seconds count, some as a duration) and exposes no accessor, so the mapping
// lives here.
var barDuration = map[quote.Period]time.Duration{
	quote.Min1:   time.Minute,
	quote.Min3:   3 * time.Minute,
	quote.Min5:   5 * time.Minute,
	quote.Min15:  15 * time.Minute,
	quote.Min30:  30 * time.Minute,
	quote.Min60:  time.Hour,
	quote.Hour2:  2 * time.Hour,
	quote.Hour4:  4 * time.Hour,
	quote.Hour6:  6 * time.Hour,
	quote.Hour8:  8 * time.Hour,
	quote.Hour12: 12 * time.Hour,
}

// tradingHoursPerDay is a regular equity session. Applying it to 24/7 crypto
// over-reaches, which is the harmless direction.
const tradingHoursPerDay = 6.5

// calendarDaysPerBar converts daily bars to calendar days. A US equity year is
// about 252 sessions to 365.25 calendar days, so this is 1.449 rather than the
// 7/5 = 1.4 that counting only weekends would suggest. The difference is market
// holidays, and getting it wrong the cheap way leaves the last few warm-up bars
// missing: 200 bars at 7/5 reaches back far enough for only 198 sessions, so an
// sma200 still opens with a zero.
const calendarDaysPerBar = 365.25 / 252

// calendarCushionPct and calendarCushionDays pad the conversion. The ratio
// above is an average, so a run whose window happens to be holiday-heavy would
// otherwise come up a bar or two short; the percentage keeps the margin growing
// with the request rather than staying flat.
const (
	calendarCushionPct  = 0.02
	calendarCushionDays = 7
)

// widenStart moves a start date back far enough to cover the given number of
// bars. It deliberately over-reaches: surplus bars are dropped once the columns
// are computed, whereas a short fetch leaves them still warming.
func widenStart(start string, bars int, period string) (string, error) {
	from, err := time.Parse(DateLayout, start)
	if err != nil {
		return "", fmt.Errorf("start_date: %w", err)
	}
	if bars <= 0 {
		return start, nil
	}
	p, err := ParsePeriod(period)
	if err != nil {
		return "", err
	}
	return from.AddDate(0, 0, -lookbackDays(bars, p)).Format(DateLayout), nil
}

// lookbackDays converts a bar count to a number of calendar days to reach back.
func lookbackDays(bars int, p quote.Period) int {
	var days float64
	switch p {
	case quote.Monthly:
		days = float64(bars) * 31
	case quote.Weekly:
		days = float64(bars) * 7
	case quote.Day3:
		days = float64(bars) * 3 * calendarDaysPerBar
	default:
		if d, ok := barDuration[p]; ok {
			days = float64(bars) * d.Hours() / tradingHoursPerDay
		} else {
			// Daily, and any period without a fixed bar length.
			days = float64(bars) * calendarDaysPerBar
		}
	}
	return int(math.Ceil(days*(1+calendarCushionPct))) + calendarCushionDays
}
