package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/markcheno/go-talib"
)

// Kind distinguishes callable indicators from the moving-average type constants
// that are passed as a "matype" argument.
type Kind string

const (
	KindFunc   Kind = "function"
	KindMAType Kind = "matype"
)

// Categories, in the order a UI should present them.
const (
	CatMAType     = "matype"
	CatOverlap    = "overlap"
	CatMomentum   = "momentum"
	CatVolume     = "volume"
	CatVolatility = "volatility"
	CatPrice      = "price"
	CatCycle      = "cycle"
	CatStatistic  = "statistic"
	CatMath       = "math"
	CatCustom     = "custom"
)

// CategoryOrder lists the categories in presentation order.
var CategoryOrder = []string{
	CatOverlap, CatMomentum, CatVolume, CatVolatility, CatPrice,
	CatCycle, CatStatistic, CatMath, CatCustom, CatMAType,
}

// FuncDef describes one symbol available to DSL expressions.
type FuncDef struct {
	Name     string   `json:"name"`
	Kind     Kind     `json:"kind"`
	Args     []string `json:"args"`
	Doc      string   `json:"doc"`
	Category string   `json:"category"`

	fn any
}

// Signature renders the call form, e.g. "sma(series,period)". Constants render
// as their bare name.
func (f FuncDef) Signature() string {
	if f.Kind == KindMAType {
		return f.Name
	}
	return f.Name + "(" + strings.Join(f.Args, ",") + ")"
}

// Desc renders the one-line description printed by -list-ta.
func (f FuncDef) Desc() string {
	return fmt.Sprintf("%s // %s", f.Signature(), f.Doc)
}

// MarshalJSON is implemented via an alias so Signature and Desc travel to the
// UI without it having to reimplement the formatting.
func (f FuncDef) MarshalJSON() ([]byte, error) {
	type alias FuncDef
	return json.Marshal(struct {
		alias
		Signature string `json:"signature"`
		Desc      string `json:"desc"`
	}{alias(f), f.Signature(), f.Desc()})
}

func fn(name, category string, args []string, doc string, impl any) FuncDef {
	return FuncDef{Name: name, Kind: KindFunc, Args: args, Doc: doc, Category: category, fn: impl}
}

func maType(name, doc string, impl any) FuncDef {
	return FuncDef{Name: name, Kind: KindMAType, Doc: doc, Category: CatMAType, fn: impl}
}

var (
	argSeries  = []string{"series"}
	argSeriesN = []string{"series", "period"}
	argHL      = []string{"h", "l"}
	argHLC     = []string{"h", "l", "c"}
	argHLCN    = []string{"h", "l", "c", "period"}
	argTwo     = []string{"series1", "series2"}
)

// catalog is the complete set of symbols bound into the DSL environment.
// Signatures and descriptions shown by -list-ta and the web UI are derived from
// these entries, so there is a single source of truth.
var catalog = []FuncDef{
	// Moving average type constants, passed as the "matype" argument.
	maType("SMA", "simple moving average type", talib.SMA),
	maType("EMA", "exponential moving average type", talib.EMA),
	maType("WMA", "weighted moving average type", talib.WMA),
	maType("DEMA", "double exponential moving average type", talib.DEMA),
	maType("TEMA", "triple exponential moving average type", talib.TEMA),
	maType("TRIMA", "triangular moving average type", talib.TRIMA),
	maType("KAMA", "Kaufman adaptive moving average type", talib.KAMA),
	maType("MAMA", "MESA adaptive moving average type", talib.MAMA),
	maType("T3MA", "triple exponential moving average type", talib.T3MA),

	// Overlap studies.
	fn("bbands", CatOverlap, []string{"series", "period", "nbdevup", "nbdevdn", "matype"}, "Bollinger Bands", talib.BBands),
	fn("dema", CatOverlap, argSeriesN, "Double Exponential Moving Average", talib.Dema),
	fn("ema", CatOverlap, argSeriesN, "Exponential Moving Average", talib.Ema),
	fn("httrendline", CatOverlap, argSeries, "Hilbert Transform - Instantaneous Trendline", talib.HtTrendline),
	fn("kama", CatOverlap, argSeriesN, "Kaufman Adaptive Moving Average", talib.Kama),
	fn("ma", CatOverlap, []string{"series", "period", "matype"}, "Moving average", talib.Ma),
	fn("mama", CatOverlap, []string{"series", "fastlimit", "slowlimit"}, "MESA Adaptive Moving Average", talib.Mama),
	fn("mavp", CatOverlap, []string{"series", "period", "minperiod", "maxperiod"}, "Moving average with variable period", talib.MaVp),
	fn("midpoint", CatOverlap, argSeriesN, "MidPoint over period", talib.MidPoint),
	fn("midprice", CatOverlap, []string{"h", "l", "period"}, "Midpoint Price over period", talib.MidPrice),
	fn("sar", CatOverlap, []string{"h", "l", "acceleration", "maximum"}, "Parabolic SAR", talib.Sar),
	fn("sarext", CatOverlap, []string{"h", "l", "startvalue", "offsetonreverse", "accelinitlong", "accellong", "accelmaxlong", "accelinitshort", "accelshort", "accelmaxshort"}, "Parabolic SAR - Extended", talib.SarExt),
	fn("sma", CatOverlap, argSeriesN, "Simple Moving Average", talib.Sma),
	fn("t3", CatOverlap, []string{"series", "period", "vfactor"}, "Triple Exponential Moving Average (T3)", talib.T3),
	fn("tema", CatOverlap, argSeriesN, "Triple Exponential Moving Average", talib.Tema),
	fn("trima", CatOverlap, argSeriesN, "Triangular Moving Average", talib.Trima),
	fn("wma", CatOverlap, argSeriesN, "Weighted Moving Average", talib.Wma),

	// Momentum indicators.
	fn("adx", CatMomentum, argHLCN, "Average Directional Movement Index", talib.Adx),
	fn("adxr", CatMomentum, argHLCN, "Average Directional Movement Index Rating", talib.AdxR),
	fn("apo", CatMomentum, []string{"series", "fast", "slow", "matype"}, "Absolute Price Oscillator", talib.Apo),
	fn("aroon", CatMomentum, []string{"h", "l", "period"}, "Aroon", talib.Aroon),
	fn("aroonosc", CatMomentum, []string{"h", "l", "period"}, "Aroon Oscillator", talib.AroonOsc),
	fn("bop", CatMomentum, []string{"o", "h", "l", "c"}, "Balance Of Power", talib.Bop),
	fn("cmo", CatMomentum, argSeriesN, "Chande Momentum Oscillator", talib.Cmo),
	fn("cci", CatMomentum, argHLCN, "Commodity Channel Index", talib.Cci),
	fn("dx", CatMomentum, argHLCN, "Directional Movement Index", talib.Dx),
	fn("macd", CatMomentum, []string{"series", "fast", "slow", "signal"}, "Moving Average Convergence/Divergence", talib.Macd),
	fn("macdext", CatMomentum, []string{"series", "fast", "slow", "matype"}, "MACD with controllable MA type", talib.MacdExt),
	fn("macdfix", CatMomentum, []string{"series", "signal"}, "MACD Fix 12/26", talib.MacdFix),
	fn("minusdi", CatMomentum, argHLCN, "Minus Directional Indicator", talib.MinusDI),
	fn("minusdm", CatMomentum, []string{"h", "l", "period"}, "Minus Directional Movement", talib.MinusDM),
	fn("mfi", CatMomentum, []string{"h", "l", "c", "v", "period"}, "Money Flow Index", talib.Mfi),
	fn("mom", CatMomentum, argSeriesN, "Momentum", talib.Mom),
	fn("plusdi", CatMomentum, argHLCN, "Plus Directional Indicator", talib.PlusDI),
	fn("plusdm", CatMomentum, []string{"h", "l", "period"}, "Plus Directional Movement", talib.PlusDM),
	fn("ppo", CatMomentum, []string{"series", "fast", "slow", "matype"}, "Percentage Price Oscillator", talib.Ppo),
	fn("rocp", CatMomentum, argSeriesN, "Rate of change Percentage: (price-prevPrice)/prevPrice", talib.Rocp),
	fn("roc", CatMomentum, argSeriesN, "Rate of change : ((price/prevPrice)-1)*100", talib.Roc),
	fn("rocr", CatMomentum, argSeriesN, "Rate of change ratio: (price/prevPrice)", talib.Rocr),
	fn("rocr100", CatMomentum, argSeriesN, "Rate of change ratio 100 scale: (price/prevPrice)*100", talib.Rocr100),
	fn("rsi", CatMomentum, argSeriesN, "Relative strength index", talib.Rsi),
	fn("stoch", CatMomentum, []string{"h", "l", "c", "fastk", "slowk", "slowkmatype", "slowd", "slowdmatype"}, "Stochastic", talib.Stoch),
	fn("stochf", CatMomentum, []string{"h", "l", "c", "fastk", "fastd", "fastmatype"}, "Stochastic Fast", talib.StochF),
	fn("stochrsi", CatMomentum, []string{"series", "period", "fastk", "fastd", "fastdmatype"}, "Stochastic Relative Strength Index", talib.StochRsi),
	fn("trix", CatMomentum, argSeriesN, "1-day Rate-Of-Change (ROC) of a Triple Smooth EMA", talib.Trix),
	fn("ultosc", CatMomentum, []string{"h", "l", "c", "period1", "period2", "period3"}, "Ultimate Oscillator", talib.UltOsc),
	fn("willr", CatMomentum, argHLCN, "Williams' %R", talib.WillR),

	// Volume indicators.
	fn("ad", CatVolume, []string{"h", "l", "c", "v"}, "Chaikin A/D Line", talib.Ad),
	fn("adosc", CatVolume, []string{"h", "l", "c", "v", "fast", "slow"}, "Chaikin A/D Oscillator", talib.AdOsc),
	fn("obv", CatVolume, []string{"series", "v"}, "On Balance Volume", talib.Obv),

	// Volatility indicators.
	fn("atr", CatVolatility, argHLCN, "Average True Range", talib.Atr),
	fn("natr", CatVolatility, argHLCN, "Normalized Average True Range", talib.Natr),
	fn("trange", CatVolatility, argHLC, "True Range", talib.TRange),

	// Price transforms.
	fn("avgprice", CatPrice, []string{"o", "h", "l", "c"}, "Average Price (o+h+l+c)/4", talib.AvgPrice),
	fn("medprice", CatPrice, argHL, "Median Price (h+l)/2", talib.MedPrice),
	fn("typprice", CatPrice, argHLC, "Typical Price (h+l+c)/3", talib.TypPrice),
	fn("wclprice", CatPrice, argHLC, "Weighted Close Price", talib.WclPrice),

	// Cycle indicators.
	fn("htdcperiod", CatCycle, argSeries, "Hilbert Transform - Dominant Cycle Period", talib.HtDcPeriod),
	fn("htdcphase", CatCycle, argSeries, "Hilbert Transform - Dominant Cycle Phase", talib.HtDcPhase),
	fn("htphasor", CatCycle, argSeries, "Hilbert Transform - Phasor Components", talib.HtPhasor),
	fn("htsine", CatCycle, argSeries, "Hilbert Transform - SineWave", talib.HtSine),
	fn("httrendmode", CatCycle, argSeries, "Hilbert Transform - Trend vs Cycle Mode", talib.HtTrendMode),

	// Statistic functions.
	fn("beta", CatStatistic, []string{"series1", "series2", "period"}, "Beta", talib.Beta),
	fn("correl", CatStatistic, []string{"series1", "series2", "period"}, "Pearson's Correlation Coefficient (r)", talib.Correl),
	fn("linearreg", CatStatistic, argSeriesN, "Linear Regression", talib.LinearReg),
	fn("linearregangle", CatStatistic, argSeriesN, "Linear Regression Angle", talib.LinearRegAngle),
	fn("linearregintercept", CatStatistic, argSeriesN, "Linear Regression Intercept", talib.LinearRegIntercept),
	fn("linearregslope", CatStatistic, argSeriesN, "Linear Regression Slope", talib.LinearRegSlope),
	fn("stddev", CatStatistic, []string{"series", "period", "nbdev"}, "Standard Deviation", talib.StdDev),
	fn("tsf", CatStatistic, argSeriesN, "Time Series Forecast", talib.Tsf),
	fn("var", CatStatistic, argSeriesN, "Variance", talib.Var),

	// Math transforms and operators.
	fn("acos", CatMath, argSeries, "Vector Trigonometric ACOS", talib.Acos),
	fn("asin", CatMath, argSeries, "Vector Trigonometric ASIN", talib.Asin),
	fn("atan", CatMath, argSeries, "Vector Trigonometric ATAN", talib.Atan),
	fn("ceil", CatMath, argSeries, "Vector CEIL", talib.Ceil),
	fn("cos", CatMath, argSeries, "Vector Trigonometric COS", talib.Cos),
	fn("cosh", CatMath, argSeries, "Vector Trigonometric COSH", talib.Cosh),
	fn("exp", CatMath, argSeries, "Vector arithmetic EXP", talib.Exp),
	fn("floor", CatMath, argSeries, "Vector FLOOR", talib.Floor),
	fn("ln", CatMath, argSeries, "Vector natural log LN", talib.Ln),
	fn("log10", CatMath, argSeries, "Vector LOG10", talib.Log10),
	fn("sin", CatMath, argSeries, "Vector Trigonometric SIN", talib.Sin),
	fn("sinh", CatMath, argSeries, "Vector Trigonometric SINH", talib.Sinh),
	fn("sqrt", CatMath, argSeries, "Vector SQRT", talib.Sqrt),
	fn("tan", CatMath, argSeries, "Vector Trigonometric TAN", talib.Tan),
	fn("tanh", CatMath, argSeries, "Vector Trigonometric TANH", talib.Tanh),
	fn("add", CatMath, argTwo, "Vector arithmetic addition", talib.Add),
	fn("max", CatMath, argSeriesN, "Highest value over a period", talib.Max),
	fn("maxindex", CatMath, argSeriesN, "Index of highest value over a specified period", talib.MaxIndex),
	fn("min", CatMath, argSeriesN, "Lowest value over a period", talib.Min),
	fn("minindex", CatMath, argSeriesN, "Index of lowest value over a specified period", talib.MinIndex),
	fn("minmax", CatMath, argSeriesN, "Lowest and highest values over a specified period", talib.MinMax),
	fn("minmaxindex", CatMath, argSeriesN, "Indexes of lowest and highest values over a specified period", talib.MinMaxIndex),
	fn("sub", CatMath, argTwo, "Vector arithmetic subtraction", talib.Sub),
	fn("sum", CatMath, argSeriesN, "Vector summation", talib.Sum),

	// go-scan additions. mult and div shadow the talib equivalents: div here
	// yields 0 rather than Inf when the divisor is 0.
	fn("and", CatCustom, argTwo, "Vector logical AND", and),
	fn("or", CatCustom, argTwo, "Vector logical OR", or),
	fn("gt", CatCustom, argTwo, "Vector greater than", gt),
	fn("lt", CatCustom, argTwo, "Vector less than", lt),
	fn("gte", CatCustom, argTwo, "Vector greater than or equal", gte),
	fn("lte", CatCustom, argTwo, "Vector less than or equal", lte),
	fn("mult", CatCustom, argTwo, "Vector multiplication", mult),
	fn("div", CatCustom, argTwo, "Vector division, 0 where the divisor is 0", div),
	fn("series", CatCustom, []string{"value", "n"}, "Create a series of n copies of value", series),
	fn("cumsum", CatCustom, argSeries, "Cumulative sum", cumsum),
	fn("normalize", CatCustom, argSeries, "Min-max normalize a series to [0,1]", normalize),
	fn("lag", CatCustom, argSeriesN, "Lag series by period", lag),
	fn("shift", CatCustom, argSeriesN, "Shift series by period, negative shifts forward", shift),
	fn("sharpe", CatCustom, []string{"returns", "window"}, "Rolling mean/variance ratio of returns", rollingSharpeRatio),
	fn("sign", CatCustom, argSeries, "Sign of series: 1, -1 or 0", sign),
}

// Catalog returns every symbol available to DSL expressions.
func Catalog() []FuncDef {
	out := make([]FuncDef, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup finds a catalog entry by name.
func Lookup(name string) (FuncDef, bool) {
	for _, f := range catalog {
		if f.Name == name {
			return f, true
		}
	}
	return FuncDef{}, false
}
