package engine

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/markcheno/go-quote"
	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
)

// EvalTimeout bounds a single expression evaluation so a pathological
// expression cannot wedge a request.
var EvalTimeout = 10 * time.Second

// baseEnv holds the catalog symbols. It is created once and never mutated
// afterwards; every evaluation runs in a child scope so concurrent evaluations
// cannot see each other's bindings.
var baseEnv = func() *env.Env {
	e := env.NewEnv()
	for _, f := range catalog {
		if err := e.Define(f.Name, f.fn); err != nil {
			panic(fmt.Sprintf("go-scan: cannot define %q: %v", f.Name, err))
		}
	}
	return e
}()

// Evaluator evaluates DSL expressions in an isolated scope. One Evaluator
// serves one ticker: columns defined by earlier expressions are visible to
// later ones, and nothing leaks to another ticker or another request.
//
// An Evaluator is not safe for concurrent use; create one per ticker.
type Evaluator struct {
	env *env.Env
}

// NewEvaluator returns an Evaluator with its own scope.
func NewEvaluator() *Evaluator {
	child := baseEnv.NewEnv()
	// Pre-declare the result symbol so assignment binds here rather than
	// searching up into the shared parent scope.
	_ = child.Define("result", nil)
	return &Evaluator{env: child}
}

// Define binds a symbol in this evaluator's scope.
func (ev *Evaluator) Define(symbol string, value any) error {
	if err := ev.env.Define(symbol, value); err != nil {
		return fmt.Errorf("cannot define %q: %w", symbol, err)
	}
	return nil
}

// BindQuote makes a ticker's OHLCV series available as d, o, h, l, c and v.
func (ev *Evaluator) BindQuote(q quote.Quote) error {
	bindings := []struct {
		name  string
		value any
	}{
		{"d", q.Date}, {"o", q.Open}, {"h", q.High},
		{"l", q.Low}, {"c", q.Close}, {"v", q.Volume},
	}
	for _, b := range bindings {
		if err := ev.Define(b.name, b.value); err != nil {
			return err
		}
	}
	return nil
}

// eval executes "result=<expr>" and returns the result value.
func (ev *Evaluator) eval(ctx context.Context, expr string) (result any, err error) {
	// The vector helpers and go-talib both index by position and panic on
	// malformed input; surface that as an error instead of taking down the
	// process.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, EvalTimeout)
	defer cancel()

	if _, err := vm.ExecuteContext(ctx, ev.env, nil, "result="+expr); err != nil {
		return nil, fmt.Errorf("execute error: %w", err)
	}
	value, err := ev.env.Get("result")
	if err != nil {
		return nil, fmt.Errorf("get error: %w", err)
	}
	return value, nil
}

// Column evaluates expr against the currently bound quote and binds the result
// under name so later expressions can refer to it.
func (ev *Evaluator) Column(ctx context.Context, name, expr string) ([]float64, error) {
	value, err := ev.eval(ctx, expr)
	if err != nil {
		return nil, err
	}
	out, ok := value.([]float64)
	if !ok {
		return nil, fmt.Errorf("expression must produce a series of numbers, got %T", value)
	}
	if err := ev.Define(name, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetColumn evaluates a single expression against a quote in a throwaway scope.
// It is a convenience wrapper for callers that only need one column.
func GetColumn(q quote.Quote, name, expr string) ([]float64, error) {
	ev := NewEvaluator()
	if err := ev.BindQuote(q); err != nil {
		return nil, err
	}
	return ev.Column(context.Background(), name, expr)
}

// reservedWords are anko builtins or keywords that collide with go-scan column
// names and so are prefixed with an underscore before evaluation.
var reservedWords = []string{"open", "high", "low", "close", "date"}

var reservedWordRe = func() *regexp.Regexp {
	return regexp.MustCompile(`\b(` + strings.Join(reservedWords, "|") + `)\b`)
}()

// escapeReservedWords prefixes reserved identifiers with an underscore. It
// matches whole identifiers only, so a column named close_ma or AAPL_close is
// left alone.
func escapeReservedWords(expr string) string {
	return reservedWordRe.ReplaceAllString(expr, "_$1")
}

// EvalFilter evaluates a boolean filter expression against one row. Values that
// parse as numbers are bound as numbers, so comparisons are numeric rather than
// lexicographic. An empty filter passes.
func EvalFilter(filter string, header, row []string) (bool, error) {
	return EvalFilterContext(context.Background(), filter, header, row)
}

// EvalFilterContext is EvalFilter with a caller-supplied context.
func EvalFilterContext(ctx context.Context, filter string, header, row []string) (bool, error) {
	if strings.TrimSpace(filter) == "" {
		return true, nil
	}

	ev := NewEvaluator()
	for i, name := range header {
		if i >= len(row) {
			break
		}
		if err := ev.Define(escapeReservedWords(name), coerce(row[i])); err != nil {
			return false, err
		}
	}

	value, err := ev.eval(ctx, escapeReservedWords(filter))
	if err != nil {
		return false, err
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	case float64:
		return v != 0, nil
	case int64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("filter must produce a boolean, got %T", value)
	}
}

// coerce converts a cell to a float64 when it looks like one, so filter
// expressions compare numerically.
func coerce(cell string) any {
	if f, err := strconv.ParseFloat(strings.TrimSpace(cell), 64); err == nil {
		return f
	}
	return cell
}
