package engine

import "fmt"

// The vector helpers below extend go-talib with the elementwise comparison,
// logic and shifting primitives the DSL needs. They all return a new slice the
// same length as their first argument.

// requireSameLen panics with a readable message rather than an index out of
// range when two series do not line up. The evaluator converts the panic into
// an error attributed to the offending column.
func requireSameLen(name string, a, b []float64) {
	if len(a) != len(b) {
		panic(fmt.Sprintf("%s: series lengths differ (%d vs %d)", name, len(a), len(b)))
	}
}

// gt performs element-wise greater-than comparison of two float64 arrays
func gt(a, b []float64) []float64 {
	requireSameLen("gt", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if a[i] > b[i] {
			result[i] = 1.0
		}
	}
	return result
}

// lt performs element-wise less-than comparison of two float64 arrays
func lt(a, b []float64) []float64 {
	requireSameLen("lt", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if a[i] < b[i] {
			result[i] = 1.0
		}
	}
	return result
}

// gte performs element-wise greater-than-or-equal-to comparison of two float64 arrays
func gte(a, b []float64) []float64 {
	requireSameLen("gte", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if a[i] >= b[i] {
			result[i] = 1.0
		}
	}
	return result
}

// lte performs element-wise less-than-or-equal-to comparison of two float64 arrays
func lte(a, b []float64) []float64 {
	requireSameLen("lte", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if a[i] <= b[i] {
			result[i] = 1.0
		}
	}
	return result
}

// mult performs element-wise multiplication of two float64 arrays
func mult(a, b []float64) []float64 {
	requireSameLen("mult", a, b)
	result := make([]float64, len(a))
	for i := range a {
		result[i] = a[i] * b[i]
	}
	return result
}

// div performs element-wise division of two float64 arrays, yielding 0 rather
// than an infinity where the divisor is zero.
func div(a, b []float64) []float64 {
	requireSameLen("div", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if b[i] != 0.0 {
			result[i] = a[i] / b[i]
		}
	}
	return result
}

// series creates a series of n copies of a float64 value
func series(value float64, n int) []float64 {
	if n < 0 {
		panic(fmt.Sprintf("series: negative length %d", n))
	}
	result := make([]float64, n)
	for i := range result {
		result[i] = value
	}
	return result
}

// and performs element-wise logical AND of two float64 arrays
func and(a, b []float64) []float64 {
	requireSameLen("and", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if a[i] == 1.0 && b[i] == 1.0 {
			result[i] = 1.0
		}
	}
	return result
}

// or performs element-wise logical OR of two float64 arrays
func or(a, b []float64) []float64 {
	requireSameLen("or", a, b)
	result := make([]float64, len(a))
	for i := range a {
		if a[i] == 1.0 || b[i] == 1.0 {
			result[i] = 1.0
		}
	}
	return result
}

// cumsum computes the cumulative sum of a float64 array
func cumsum(a []float64) []float64 {
	result := make([]float64, len(a))
	sum := 0.0
	for i := range a {
		sum += a[i]
		result[i] = sum
	}
	return result
}

// normalize min-max scales a float64 array into [0,1]. A constant series
// normalizes to all zeros rather than NaN.
func normalize(a []float64) []float64 {
	result := make([]float64, len(a))
	if len(a) == 0 {
		return result
	}
	min, max := a[0], a[0]
	for i := range a {
		if a[i] < min {
			min = a[i]
		}
		if a[i] > max {
			max = a[i]
		}
	}
	if max == min {
		return result
	}
	for i := range a {
		result[i] = (a[i] - min) / (max - min)
	}
	return result
}

// lag lags a float64 array by a given period
func lag(a []float64, period int) []float64 {
	return shift(a, period)
}

// shift shifts a float64 array forward or backward by a given period.
// Forward shifts have negative periods, backward shifts have positive periods.
func shift(a []float64, period int) []float64 {
	result := make([]float64, len(a))
	n := len(a)

	for i := range a {
		newIndex := i - period
		if newIndex >= 0 && newIndex < n {
			result[i] = a[newIndex]
		}
	}
	return result
}

// rollingSharpeRatio computes a rolling mean/variance ratio over the trailing
// window. Note this divides by the variance, not the standard deviation, and is
// not annualized; it is kept as-is for compatibility with existing configs.
func rollingSharpeRatio(returns []float64, window int) []float64 {
	result := make([]float64, len(returns))
	if window <= 0 {
		panic(fmt.Sprintf("sharpe: window must be positive, got %d", window))
	}
	for i := range returns {
		if i < window {
			continue
		}
		// Compute the mean
		sum := 0.0
		for j := i - window; j < i; j++ {
			sum += returns[j]
		}
		mean := sum / float64(window)
		// Compute the variance
		sum = 0.0
		for j := i - window; j < i; j++ {
			sum += (returns[j] - mean) * (returns[j] - mean)
		}
		variance := sum / float64(window)
		if variance != 0 {
			result[i] = mean / variance
		}
	}
	return result
}

// sign returns 1 for positive values, -1 for negative values and 0 for zero
func sign(a []float64) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		if a[i] > 0 {
			result[i] = 1.0
		} else if a[i] < 0 {
			result[i] = -1.0
		}
	}
	return result
}
