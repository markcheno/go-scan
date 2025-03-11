# TIDE: Technical Indicator & Data Engine

## 1. Overview

TIDE (Technical Indicator & Data Engine) is a simple, practical language for manipulating financial stock data, focusing on historical OHLCV data processing, technical analysis, and preparation of data for backtesting and research. TIDE supports working with individual stocks and groups of stocks (markets), with a straightforward syntax designed for financial analysts and traders.

## 2. Core Design Principles

- **Straightforward**: Simple, intuitive syntax focused on practical use
- **Columnar**: Data is organized in named columns that can be referenced in expressions
- **Composable**: Expressions can be combined to create complex calculations
- **Minimalistic**: Few keywords and consistent syntax patterns

## 3. Basic Syntax

```tide
// Comments start with double slash
aapl = load_tiingo("AAPL", from="2020-01-01", to="2023-01-01")  // Load data
aapl.sma20 = sma(close, 20)                                     // Add technical indicator
aapl.signal = close > aapl.sma20                                // Add computed column
aapl.save_csv("aapl_data.csv")                                  // Save results
```

## 4. Data Structures

### Single Stock DataFrames

The core data structure in TIDE is a DataFrame-like object with named columns:

- Standard OHLCV columns: `date` (or `d`), `open` (or `o`), `high` (or `h`), `low` (or `l`), `close` (or `c`), `volume` (or `v`)
- Additional computed columns added through operations

### Markets (Stock Groups)

Collections of multiple stock symbols:

```tide
nasdaq100 = market("nasdaq100", from="2020-01-01", to="2023-01-01")
nasdaq100.sma50 = sma(close, 50)  // Apply to all stocks
```

## 5. Data Sources and I/O

### Loading Data

```tide
// Load single stock
aapl = load_tiingo("AAPL", from="2020-01-01", to="2023-01-01")
aapl = load_csv("path/to/aapl.csv")
aapl = load_json("path/to/aapl.json")

// Load multiple stocks (market)
tech = market(symbols=["AAPL", "MSFT", "GOOGL", "META"], from="2020-01-01", to="2023-01-01")

// Load predefined markets
nasdaq100 = market("nasdaq100", from="2020-01-01", to="2023-01-01", source="tiingo")
```

### Saving Data

```tide
// Save to file
aapl.save_csv("aapl_data.csv")
aapl.save_json("aapl_data.json")
tech.save_csv("tech_stocks.csv")
```

## 6. Expressions and Operations

### Column References

```tide
// Standard column names
aapl.close       // Full name
c                // Shorthand for current dataframe's close column

// Column shorthands
o                // open
h                // high
l                // low
c                // close
v                // volume
d                // date
```

### Arithmetic Operations

```tide
aapl.ratio = close / open
aapl.range = high - low
aapl.dollar_volume = close * volume
```

### Comparison Operations

```tide
aapl.is_up = close > open
aapl.gap_up = open > close.shift(1)    // Compare with previous day
```

### Logical Operations

```tide
aapl.buy_signal = (aapl.sma50 > aapl.sma200) & (aapl.rsi14 < 70)  // AND
aapl.sell_signal = (aapl.sma50 < aapl.sma200) | (aapl.rsi14 > 70)  // OR
aapl.neutral = ~(aapl.buy_signal | aapl.sell_signal)               // NOT
```

### Conditional Operations

```tide
aapl.position = aapl.sma50 > aapl.sma200 ? 1 : -1  // Ternary operator
```

### Technical Indicators

```tide
// Add indicators as new columns
aapl.sma20 = sma(close, 20)                  // Simple Moving Average on close prices
aapl.rsi14 = rsi(close, 14)                  // RSI with period 14
aapl.upper, aapl.middle, aapl.lower = bbands(close, 20, 2)  // Bollinger Bands

// Multiple indicators
aapl.sma50 = sma(close, 50)
aapl.sma200 = sma(close, 200)
aapl.macd, aapl.macd_signal, aapl.macd_hist = macd(close, 12, 26, 9)
```

### Data Filtering

```tide
aapl_filtered = aapl.filter(aapl.sma50 != 0 & aapl.sma200 != 0)  // Filter rows where condition is true
aapl_2022 = aapl.filter(date >= "2022-01-01" & date <= "2022-12-31")  // Filter by date range
aapl_tail = aapl.tail(100)  // Last 100 rows
aapl_head = aapl.head(50)   // First 50 rows
```

### Data Selection

```tide
aapl_subset = aapl.select(["date", "close", "sma20"])  // Select specific columns
```

## 7. Data Transformation

### Pivoting Data

```tide
// Convert market data from rows by (symbol, date) to rows by date with symbols as columns
// This transforms from long format to wide format
tech_pivot = tech.pivot(index="date", columns="symbol", values="close")

// Result has columns like AAPL_close, MSFT_close, etc.
// Can specify multiple value columns
market_data = nasdaq100.pivot(index="date", columns="symbol", values=["close", "volume"])

// Access pivoted data
aapl_prices = market_data["AAPL_close"]
msft_volumes = market_data["MSFT_volume"]

// Create a correlation matrix of stock returns
returns = market_data.pct_change()
corr_matrix = returns.correlation()
```

### Train/Test Splitting

```tide
// Split by date
aapl_train, aapl_test = aapl.split_date("2022-01-01")  // Before/after specified date

// Split by percentage (70% train, 30% test)
aapl_train, aapl_test = aapl.split_pct(0.7)

// Walk-forward testing windows
// Creates a list of (train, test) pairs for walk-forward optimization
windows = aapl.walk_forward(train_size=252, test_size=63, step=63)  // 252 days train, 63 days test, stepping forward 63 days each time
```

## 8. Market Operations

### Market-wide Operations

```tide
// Apply operations to all stocks in a market
nasdaq100.sma50 = sma(close, 50)              // Add SMA to all stocks

// Get average value across market
avg_pe = nasdaq100.pe.mean()                   // Market average P/E ratio
```

### Cross-stock Operations

```tide
// Create relative performance metric
nasdaq100.rel_to_spy = nasdaq100.close / spy.close  // Relative to S&P 500

// Create market-cap weighted index
nasdaq_index = nasdaq100.close * nasdaq100.weight   // Where weight column contains market cap weights
```

### Market Filtering

```tide
// Filter market by condition
strong_stocks = nasdaq100.filter(close > sma200)  // Only stocks above 200-day SMA
```

### Market Aggregation

```tide
// Aggregate market data
total_volume = nasdaq100.volume.sum()              // Total market volume
advancing = nasdaq100.filter(close > open).count() // Number of advancing stocks
```

## 9. Implementation Details

### Go Implementation

TIDE will be implemented in Go, leveraging:

- github.com/markcheno/go-quote for data fetching
- github.com/markcheno/go-talib for technical analysis

Core components:

1. **Lexer/Parser**: To translate TIDE code into executable operations
2. **Expression Evaluator**: To process expressions on data frames
3. **Data Frame Implementation**: To store and manipulate the stock data
4. **Technical Analysis Library**: Integration with go-talib for all indicators

### File Extension

TIDE scripts use the `.tide` file extension:

```
strategy.tide
backtest.tide
```

### Command Line Interface

TIDE scripts can be executed via the command line:

```
tide run strategy.tide
tide backtest strategy.tide --from=2020-01-01 --to=2023-01-01
```

### Integration with TA-Lib

All TA-Lib functions will be directly exposed as functions for column assignments:

- Overlap Studies: `sma`, `ema`, `bbands`, etc.
- Momentum Indicators: `rsi`, `macd`, `stoch`, etc.
- Volume Indicators: `obv`, `ad`, etc.
- Volatility Indicators: `atr`, etc.
- Pattern Recognition
- Cycle Indicators
- Price Transform
- Math Transform
- Math Operators

## 10. Example TIDE Programs

### Simple Moving Average Crossover

```tide
// Load Apple stock data
aapl = load_tiingo("AAPL", from="2020-01-01", to="2023-01-01")

// Calculate moving averages
aapl.sma50 = sma(close, 50)
aapl.sma200 = sma(close, 200)

// Calculate crossover signal
aapl.signal = aapl.sma50 > aapl.sma200 ? 1 : -1

// Filter out data before both SMAs are available
aapl_valid = aapl.filter(aapl.sma50 != 0 & aapl.sma200 != 0)

// Save the result
aapl_valid.save_csv("aapl_sma_crossover.csv")
```

### Market Analysis

```tide
// Define and load tech stocks
tech = market(symbols=["AAPL", "MSFT", "GOOGL", "AMZN", "META"], from="2022-01-01", to="2023-01-01")

// Load S&P 500 for reference
spy = load_tiingo("SPY", from="2022-01-01", to="2023-01-01")

// Calculate RSI for all tech stocks
tech.rsi14 = rsi(close, 14)

// Calculate relative strength vs SPY
first_day_close = close.shift(len(close)-1)  // First day close price
tech.rel_strength = close / first_day_close * 100 / (spy.close / spy.first_day_close * 100)

// Find outperforming stocks
outperformers = tech.filter(tech.rel_strength > 1)

// Calculate average RSI of outperformers
avg_rsi = outperformers.rsi14.mean()

// Save results
tech.save_csv("tech_analysis.csv")
```

### Sector Rotation Analysis

```tide
// Define sector ETFs
sectors = market(symbols=["XLF", "XLK", "XLE", "XLV", "XLP", "XLI", "XLY", "XLU", "XLB", "XLRE"],
                from="2020-01-01", to="2023-01-01")

// Calculate 50-day momentum
sectors.roc50 = roc(close, 50)

// Rank sectors by momentum
sectors.rank = sectors.roc50.rank(descending=true)

// Select top 3 momentum sectors
top_sectors = sectors.filter(sectors.rank <= 3)

// Save rankings
sectors.select(["symbol", "roc50", "rank"]).save_csv("sector_momentum.csv")
```

### Cross-Sectional Analysis with Pivoting

```tide
// Load S&P 500 stocks
sp500 = market("sp500", from="2022-01-01", to="2023-01-01")

// Calculate metrics
sp500.pe = close / sp500.earnings
sp500.pb = close / sp500.book_value
sp500.mkt_cap = close * sp500.shares_outstanding

// Pivot data to have one row per date with all stocks as columns
sp500_wide = sp500.pivot(index="date", columns="symbol", values=["close", "pe", "pb", "mkt_cap"])

// For the most recent date, find the 10 lowest P/E stocks
latest_date = sp500_wide.date.tail(1)
latest_data = sp500_wide.filter(date == latest_date)
low_pe_stocks = latest_data.sort_by("pe").head(10)

// Save the result
low_pe_stocks.save_csv("low_pe_stocks.csv")
```

### Train-Test Strategy Validation

```tide
// Load data
aapl = load_tiingo("AAPL", from="2018-01-01", to="2023-01-01")

// Prepare indicators
aapl.sma20 = sma(close, 20)
aapl.sma50 = sma(close, 50)
aapl.rsi14 = rsi(close, 14)

// Create strategy signal
aapl.signal = (aapl.sma20 > aapl.sma50) & (aapl.rsi14 < 70) ? 1 : 0

// Split into training and testing sets
aapl_train, aapl_test = aapl.split_date("2022-01-01")

// Calculate returns based on signal
aapl_train.strategy_return = aapl_train.signal.shift(1) * aapl_train.close.pct_change()
aapl_test.strategy_return = aapl_test.signal.shift(1) * aapl_test.close.pct_change()

// Calculate performance metrics
train_sharpe = aapl_train.strategy_return.sharpe_ratio(252)
test_sharpe = aapl_test.strategy_return.sharpe_ratio(252)

// Print results
print("Training Sharpe Ratio: ", train_sharpe)
print("Testing Sharpe Ratio: ", test_sharpe)
```

### Efficient Data Pipeline with Caching

```tide
// Create a function for reusable analysis
def analyze_stock(symbol, years=3):
    stock = load_tiingo(symbol, years=years)

    // Calculate indicators
    stock.sma50 = sma(close, 50)
    stock.sma200 = sma(close, 200)
    stock.rsi14 = rsi(close, 14)

    return stock

// Analyze multiple stocks efficiently using cache
aapl = analyze_stock("AAPL")
msft = analyze_stock("MSFT")
amzn = analyze_stock("AMZN")

// Combine for comparative analysis
stocks = [aapl, msft, amzn]
performance = {}

for stock in stocks:
    // Only calculate for the last year
    last_year = stock.filter(date >= date_offset("today", years=-1))
    performance[stock.symbol] = {
        "return": last_year.close.pct_change().cum_product() - 1,
        "max_drawdown": last_year.close.drawdown().min(),
        "volatility": last_year.close.pct_change().std() * sqrt(252)
    }

// Output results
print(performance)
```

## 11. Future Extensions for TIDE

Potential areas for expansion:

- Interactive visualization capabilities
- Backtesting frameworks
- Performance optimizations for large datasets
- Support for advanced statistical functions
- Streaming data processing
- Web/UI interface
- Integration with machine learning libraries
- Portfolio optimization tools
- Event-driven trading capabilities
