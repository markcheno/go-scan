package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/markcheno/go-quote"
)

func TestComparisonFunctions(t *testing.T) {
	tests := []struct {
		name     string
		fn       func([]float64, []float64) []float64
		a        []float64
		b        []float64
		expected []float64
	}{
		{
			name:     "gt basic comparison",
			fn:       gt,
			a:        []float64{2, 1, 3},
			b:        []float64{1, 1, 4},
			expected: []float64{1, 0, 0},
		},
		{
			name:     "lt basic comparison",
			fn:       lt,
			a:        []float64{2, 1, 3},
			b:        []float64{3, 1, 3},
			expected: []float64{1, 0, 0},
		},
		{
			name:     "gte basic comparison",
			fn:       gte,
			a:        []float64{2, 1, 3},
			b:        []float64{1, 1, 2},
			expected: []float64{1, 1, 1},
		},
		{
			name:     "lte basic comparison",
			fn:       lte,
			a:        []float64{2, 1, 3},
			b:        []float64{3, 1, 4},
			expected: []float64{1, 1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.a, tt.b)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestArrayManipulationFunctions(t *testing.T) {
	tests := []struct {
		name     string
		fn       interface{}
		input    []float64
		period   int
		expected []float64
	}{
		{
			name:     "cumsum basic",
			fn:       cumsum,
			input:    []float64{1, 2, 3, 4},
			expected: []float64{1, 3, 6, 10},
		},
		{
			name:     "normalize basic",
			fn:       normalize,
			input:    []float64{1, 2, 3, 4},
			expected: []float64{0, 1.0 / 3.0, 2.0 / 3.0, 1},
		},
		{
			name:     "lag with period 1",
			fn:       lag,
			input:    []float64{1, 2, 3, 4},
			period:   1,
			expected: []float64{0, 1, 2, 3},
		},
		{
			name:     "shift forward",
			fn:       shift,
			input:    []float64{1, 2, 3, 4},
			period:   -1,
			expected: []float64{2, 3, 4, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result []float64
			switch fn := tt.fn.(type) {
			case func([]float64) []float64:
				result = fn(tt.input)
			case func([]float64, int) []float64:
				result = fn(tt.input, tt.period)
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetColumn(t *testing.T) {
	// Create a sample quote
	q := quote.Quote{
		Symbol: "TEST",
		Date:   []time.Time{time.Now(), time.Now().Add(24 * time.Hour)},
		Open:   []float64{100, 101},
		High:   []float64{102, 103},
		Low:    []float64{98, 99},
		Close:  []float64{101, 102},
		Volume: []float64{1000, 1100},
	}

	tests := []struct {
		name        string
		column      string
		expr        string
		shouldError bool
	}{
		{
			name:        "simple sma",
			column:      "sma2",
			expr:        "sma(c,2)",
			shouldError: false,
		},
		{
			name:        "invalid expression",
			column:      "invalid",
			expr:        "invalid(c)",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetColumn(q, tt.column, tt.expr)
			if (err != nil) != tt.shouldError {
				t.Errorf("GetColumn() error = %v, shouldError %v", err, tt.shouldError)
				return
			}
			if !tt.shouldError && result == nil {
				t.Error("expected non-nil result for valid expression")
			}
		})
	}
}

func TestEvalFilter(t *testing.T) {
	tests := []struct {
		name        string
		filter      string
		header      []string
		row         []string
		expected    bool
		shouldError bool
	}{
		{
			name:        "empty filter",
			filter:      "",
			header:      []string{"close"},
			row:         []string{"100"},
			expected:    true,
			shouldError: false,
		},
		{
			name:        "simple comparison",
			filter:      "close > 50",
			header:      []string{"close"},
			row:         []string{"100"},
			expected:    true,
			shouldError: false,
		},
		{
			name:        "invalid expression",
			filter:      "invalid syntax",
			header:      []string{"close"},
			row:         []string{"100"},
			expected:    false,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvalFilter(tt.filter, tt.header, tt.row)
			if (err != nil) != tt.shouldError {
				t.Errorf("EvalFilter() error = %v, shouldError %v", err, tt.shouldError)
				return
			}
			if !tt.shouldError && result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}
