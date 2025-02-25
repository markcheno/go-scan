package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/markcheno/go-quote"
	"github.com/markcheno/go-talib"
	"github.com/mattn/anko/env"
	_ "github.com/mattn/anko/packages"
	"github.com/mattn/anko/vm"
)

var e *env.Env

// funcDefinition is a struct that holds the name, function and description of a function
type funcDefinition struct {
	Name string
	Fn   interface{}
	Desc string
}

// TA functions from go-talib
var taFunctions = []funcDefinition{
	{"SMA", talib.SMA, "SMA // simple moving average type"},
	{"EMA", talib.EMA, "EMA // exponential moving average type"},
	{"WMA", talib.WMA, "WMA // weighted moving average type"},
	{"DEMA", talib.DEMA, "DEMA // double exponential moving average type"},
	{"TEMA", talib.TEMA, "TEMA // triple exponential moving average type"},
	{"TRIMA", talib.TRIMA, "TRIMA // triangular moving average type"},
	{"KAMA", talib.KAMA, "KAMA // Kaufman adaptive moving average type"},
	{"MAMA", talib.MAMA, "MAMA // MESA adaptive moving average type"},
	{"T3MA", talib.T3MA, "T3MA // triple exponential moving average"},
	{"bbands", talib.BBands, "bbands(series,period,nbdevup,nbdevdn,matype) // Bollinger Bands"},
	{"dema", talib.Dema, "dema(series,period) // Double Exponential Moving Average"},
	{"ema", talib.Ema, "ema(series,period) // Exponential Moving Average"},
	{"httrendline", talib.HtTrendline, "httrendline(series) //Hilbert Transform - Instantaneous Trendline"},
	{"kama", talib.Kama, "kama(series,period) // Kaufman Adaptive Moving Average"},
	{"ma", talib.Ma, "ma(series,period,matype) // Moving average"},
	{"mama", talib.Mama, "mama(series,fastlimit,slowlimit) // MESA Adaptive Moving Average"},
	{"mavp", talib.MaVp, "mavp(series,period,minperiod,maxperiod) // Moving average with variable period"},
	{"midpoint", talib.MidPoint, "midpoint(series,period) // MidPoint over period"},
	{"midprice", talib.MidPrice, "midprice(h,l,period) // Midpoint Price over period"},
	{"sar", talib.Sar, "sar(h,l,acceleration,maximum) // Parabolic SAR"},
	{"sarext", talib.SarExt, "sarext(h,l,startvalue,offsetonreverse,accelinitlong,accellong,accelmaxlong,accelinitshort,accelshort,accelmaxshort) // Parabolic SAR - Extended"},
	{"sma", talib.Sma, "sma(series,period) // Simple Moving Average"},
	{"t3", talib.T3, "t3(series,period,vfactor) // Triple Exponential Moving Average (T3)"},
	{"tema", talib.Tema, "tema(series,period) // Triple Exponential Moving Average"},
	{"trima", talib.Trima, "trima(series,period) // Triangular Moving Average"},
	{"wma", talib.Wma, "wma(series,period) // Weighted Moving Average"},
	{"adx", talib.Adx, "adx(h,l,c,period) // Average Directional Movement Index"},
	{"adxr", talib.AdxR, "adxr(h,l,c,period) // Average Directional Movement Index Rating"},
	{"apo", talib.Apo, "apo(series,fast,slow,matype) // Absolute Price Oscillator"},
	{"aroon", talib.Aroon, "aroon(h,l,period) // Aroon"},
	{"aroonosc", talib.AroonOsc, "aroonosc(h,l,period) // Aroon Oscillator"},
	{"bop", talib.Bop, "bop(o,h,l,c) // Balance Of Power"},
	{"cmo", talib.Cmo, "cmo(series,period) // Chande Momentum Oscillator"},
	{"cci", talib.Cci, "cci(h,l,c,period) // Commodity Channel Index"},
	{"dx", talib.Dx, "dx(h,l,c,period) // Directional Movement Index"},
	{"macd", talib.Macd, "macd(series,fast,slow,signal) // Moving Average Convergence/Divergence"},
	{"macdext", talib.MacdExt, "macdext(series,fast,slow,matype) // MACD with controllable MA type"},
	{"macdfix", talib.MacdFix, "macdfix(series,signal) // MACD Fix 12/26"},
	{"minusdi", talib.MinusDI, "minusdi(h,l,c,period) // Minus Directional Indicator"},
	{"minusdm", talib.MinusDM, "minusdm(h,l,period) // Minus Directional Movement"},
	{"mfi", talib.Mfi, "mfi(h,l,c,v,period) // Money Flow Index"},
	{"mom", talib.Mom, "mom(series,period) // Momentum"},
	{"plusdi", talib.PlusDI, "plusdi(h,l,c,period) // Plus Directional Indicator"},
	{"plusdm", talib.PlusDM, "plusdm(h,l,period) // Plus Directional Movement"},
	{"ppo", talib.Ppo, "ppo(series,fast,slow,matype) // Percentage Price Oscillator"},
	{"rocp", talib.Rocp, "rocp(series,period) // Rate of change Percentage: (price-prevPrice)/prevPrice"},
	{"roc", talib.Roc, "roc(series,period) // Rate of change : ((price/prevPrice)-1)*100"},
	{"rocr", talib.Rocr, "rocr(series,period) // Rate of change ratio: (price/prevPrice)"},
	{"rocr100", talib.Rocr100, "rocr100(series,period) // Rate of change ratio 100 scale: (price/prevPrice)*100"},
	{"rsi", talib.Rsi, "rsi(series,period) // Relative strength index"},
	{"stoch", talib.Stoch, "stoch(h,l,c,fastk,slowk,slowkmatype,slowd,slowdmatype) // Stochastic"},
	{"stochf", talib.StochF, "stochf(h,l,c,fastk,fastd,fastmatype) // Stochastic Fast"},
	{"stochrsi", talib.StochRsi, "stochrsi(series,period,fastk,fastd,fastdmatype) // Stochastic Relative Strength Index"},
	{"trix", talib.Trix, "trix(series,period) // 1-day Rate-Of-Change (ROC) of a Triple Smooth EMA"},
	{"ultosc", talib.UltOsc, "ultosc(h,l,c,period1,period2,period3) // Ultimate Oscillator"},
	{"willr", talib.WillR, "willr(h,l,c,period) // Williams' %R"},
	{"ad", talib.Ad, "ad(h,l,c,v) // Chaikin A/D Line"},
	{"adosc", talib.AdOsc, "adosc(h,l,c,v,fast,slow) // Chaikin A/D Oscillator"},
	{"obv", talib.Obv, "obv(series,v) // On Balance Volume"},
	{"atr", talib.Atr, "atr(h,l,c,period) // Average True Range"},
	{"natr", talib.Natr, "natr(h,l,c,period) // Normalized Average True Range"},
	{"trange", talib.TRange, "trange(h,l,c) // True Range"},
	{"avgprice", talib.AvgPrice, "avgprice(o,h,l,c) // Average Price (o+h+l+c)/4"},
	{"medprice", talib.MedPrice, "medprice(h,l) // Median Price (h+l)/2"},
	{"typprice", talib.TypPrice, "typprice(h,l,c) // Typical Price (h+l+c)/3"},
	{"wclprice", talib.WclPrice, "wclprice(h,l,c) // Weighted Close Price"},
	{"htdcperiod", talib.HtDcPeriod, "htdcperiod(series) // Hilbert Transform - Dominant Cycle Period"},
	{"htdcphase", talib.HtDcPhase, "htdcphase(series) // Hilbert Transform - Dominant Cycle Phase"},
	{"htphasor", talib.HtPhasor, "htphasor(series) // Hilbert Transform - Phasor Components"},
	{"htsine", talib.HtSine, "htsine(series) // Hilbert Transform - SineWave"},
	{"httrendmode", talib.HtTrendMode, "htrrendmode(series) // Hilbert Transform - Trend vs Cycle Mode"},
	{"beta", talib.Beta, "beta(series1,series2,period) // Beta"},
	{"correl", talib.Correl, "correl(series1,series2,period) // Pearson's Correlation Coefficient (r)"},
	{"linearreg", talib.LinearReg, "linearreg(series,period) // Linear Regression"},
	{"linearregangle", talib.LinearRegAngle, "linerarregangle(series,period) // Linear Regression Angle"},
	{"linearregintercept", talib.LinearRegIntercept, "linearregintercept(series,period) // Linear Regression Intercept"},
	{"linearregslope", talib.LinearRegSlope, "linearregslope(series,period) // Linear Regression Slope"},
	{"stddev", talib.StdDev, "stddev(series,period,nbdec) // Standard Deviation"},
	{"tsf", talib.Tsf, "tsf(series,period) // Time Series Forecast"},
	{"var", talib.Var, "var(series,period) // Variance"},
	{"acos", talib.Acos, "acos(series) // Vector Trigonometric ACOS"},
	{"asin", talib.Asin, "asin(series) // Vector Trigonometric ASIN"},
	{"atan", talib.Atan, "atan(series) // Vector Trigonometric ATAN"},
	{"ceil", talib.Ceil, "ceil(series) // Vector CEIL"},
	{"cos", talib.Cos, "cos(series) // Vector Trigonometric COS"},
	{"cosh", talib.Cosh, "cosh(series) // Vector Trigonometric COSH"},
	{"exp", talib.Exp, "exp(series) // Vector arithmetic EXP"},
	{"floor", talib.Floor, "floor(series) // Vector FLOOR"},
	{"ln", talib.Ln, "ln(series) // Vector natural log LN"},
	{"log10", talib.Log10, "log10(series) // Vector LOG10"},
	{"sin", talib.Sin, "sin(series) // Vector Trigonometric SIN"},
	{"sinh", talib.Sinh, "sinh(series) // Vector Trigonometric SINH"},
	{"sqrt", talib.Sqrt, "sqrt(series) // Vector SQRT"},
	{"tan", talib.Tan, "tan(series) // Vector Trigonometric TAN"},
	{"tanh", talib.Tanh, "tanh(series) // Vector Trigonometric TANH"},
	{"add", talib.Add, "add(series1,series2) // Vector arithmetic addition"},
	{"div", talib.Div, "div(series1,series2) // Vector arithmetic division"},
	{"max", talib.Max, "max(series,period) // Highest value over a period"},
	{"maxindex", talib.MaxIndex, "maxindex(series,period) // Index of highest value over a specified period"},
	{"min", talib.Min, "min(series,period) // Lowest value over a period"},
	{"minindex", talib.MinIndex, "minindex(series,period) // Index of lowest value over a specified period"},
	{"minmax", talib.MinMax, "minmax(series,period) // Lowest and highest values over a specified period"},
	{"minmaxindex", talib.MinMaxIndex, "minmaxindex(series,period) // Indexes of lowest and highest values over a specified period"},
	{"mult", talib.Mult, "mult(series1,series2) // Vector arithmetic multiply"},
	{"sub", talib.Sub, "sub(series1,series2) // Vector arithmetic subtraction"},
	{"sum", talib.Sum, "sum(series,period) // Vector summation"},
	{"gt", gt, "gt(series1,series2) // Vector greater than"},
	{"lt", lt, "lt(series1,series2) // Vector less than"},
	{"gte", gte, "gte(series1,series2) // Vector greater than or equal"},
	{"lte", lte, "lte(series1,series2) // Vector less than or equal"},
	{"cumsum", cumsum, "cumsum(series) // Cumulative sum"},
	{"normalize", normalize, "normalize(series) // Normalize series"},
	{"lag", lag, "lag(series,period) // Lag series by period"},
	{"shift", shift, "shift(series,period) // Shift series by period"},
}

// Suplementary functions

// gt performs element-wise greater-than comparison of two float64 arrays
func gt(a, b []float64) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		if a[i] > b[i] {
			result[i] = 1.0
		} else {
			result[i] = 0.0
		}
	}
	return result
}

// lt performs element-wise less-than comparison of two float64 arrays
func lt(a, b []float64) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		if a[i] < b[i] {
			result[i] = 1.0
		} else {
			result[i] = 0.0
		}
	}
	return result
}

// gte performs element-wise greater-than-or-equal-to comparison of two float64 arrays
func gte(a, b []float64) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		if a[i] >= b[i] {
			result[i] = 1.0
		} else {
			result[i] = 0.0
		}
	}
	return result
}

// lte performs element-wise less-than-or-equal-to comparison of two float64 arrays
func lte(a, b []float64) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		if a[i] <= b[i] {
			result[i] = 1.0
		} else {
			result[i] = 0.0
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

// normalize normalizes a float64 array
func normalize(a []float64) []float64 {
	result := make([]float64, len(a))
	min := a[0]
	max := a[0]
	for i := range a {
		if a[i] < min {
			min = a[i]
		}
		if a[i] > max {
			max = a[i]
		}
	}
	for i := range a {
		result[i] = (a[i] - min) / (max - min)
	}
	return result
}

// lag lags a float64 array by a given period
func lag(a []float64, period int) []float64 {
	result := make([]float64, len(a))
	for i := range a {
		if i-period >= 0 {
			result[i] = a[i-period]
		} else {
			result[i] = 0.0
		}
	}
	return result
}

// shift shifts a float64 array forward or backward by a given period
// Forward shifts have negative periods, backward shifts have positive periods
func shift(a []float64, period int) []float64 {
	result := make([]float64, len(a))
	n := len(a)

	for i := range a {
		newIndex := i - period
		if newIndex >= 0 && newIndex < n {
			result[i] = a[newIndex]
		} else {
			result[i] = 0.0
		}
	}
	return result
}

// define defines a symbol in the environment
func define(symbol string, value interface{}) {
	err := e.Define(symbol, value)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// init initializes the environment and defines the functions
func init() {
	e = env.NewEnv()
	for _, f := range taFunctions {
		define(f.Name, f.Fn)
	}
}

// GetTA returns the list of technical analysis functions
func GetTA() []funcDefinition {
	return taFunctions
}

// GetColumn returns the result of evaluating an expression on a quote
func GetColumn(quote quote.Quote, column, expr string) ([]float64, error) {

	define("d", quote.Date)
	define("o", quote.Open)
	define("h", quote.High)
	define("l", quote.Low)
	define("c", quote.Close)
	define("v", quote.Volume)

	_, err := vm.Execute(e, nil, "result="+expr)
	if err != nil {
		return nil, fmt.Errorf("execute error: %v", err)
	}

	var results interface{}
	results, err = e.Get("result")
	if err != nil {
		return nil, fmt.Errorf("get error: %v", err)
	}
	exportedResults, ok := results.([]float64)
	if !ok {
		return nil, fmt.Errorf("type assertion error: %v", err)
	}
	// Define the column
	define(column, exportedResults)

	return exportedResults, nil
}

func replaceReservedWord(expr, word string) string {
	return strings.ReplaceAll(expr, word, fmt.Sprintf("_%v", word))
}

func replaceReservedWords(expr string) string {
	expr = replaceReservedWord(expr, "open")
	expr = replaceReservedWord(expr, "high")
	expr = replaceReservedWord(expr, "low")
	expr = replaceReservedWord(expr, "close")
	expr = replaceReservedWord(expr, "date")
	return expr
}

// EvalFilter evaluates a filter expression on a row
func EvalFilter(filter string, header, row []string) (bool, error) {

	if filter == "" {
		return true, nil
	}
	filter = replaceReservedWords(filter)

	// Define the variables
	for idx, name := range header {
		name = replaceReservedWords(name)
		//fmt.Printf("defining: name: %v, value: %v\n", name, row[idx])
		define(string(name), row[idx])
	}

	// Execute the expression
	_, err := vm.Execute(e, nil, "result="+filter)
	if err != nil {
		//fmt.Printf("filter: %v\n", filter)
		return false, fmt.Errorf("execute error: %v", err)
	}

	// Get the result
	var results interface{}
	results, err = e.Get("result")
	if err != nil {
		return false, fmt.Errorf("get error: %v", err)
	}
	exportedResults, ok := results.(bool)
	if !ok {
		return false, fmt.Errorf("type assertion error: %v", err)
	}
	return exportedResults, nil
}
