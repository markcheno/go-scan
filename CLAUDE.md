# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

go-scan is a Go application for fetching OHLCV (Open, High, Low, Close, Volume) data for stock tickers, calculating technical indicators, filtering data, and writing results to CSV and/or Parquet files. The application supports multiple data sources and allows flexible configuration through command-line flags and YAML configuration files.

## Build and Test Commands

```bash
# Build the application
go build -o scan

# Run tests
go test -v

# Run a specific test
go test -v -run TestName

# Install dependencies
go mod tidy
```

## Running the Application

```bash
# Basic usage with command-line flags
scan -tickers=AAPL,GOOG,MSFT -start=2024-01-01 -end=2024-12-31 -outfile=output.csv -columns="sma20=sma(c,20)|rsi2=rsi(c,2)"

# Using a YAML configuration file
scan -config=config.yaml

# List available markets
scan -list-markets

# List available technical analysis functions
scan -list-ta
```

## Architecture

### Core Components

**main.go**: Entry point and orchestration
- Command-line flag parsing and YAML configuration loading
- Fetches OHLCV data from multiple sources (Yahoo, Tiingo, Coinbase)
- Iterates through tickers, applies technical indicators, and filters results
- Pivots data and splits into training/testing sets based on configuration
- Writes results to CSV files

**dsl.go**: Domain-Specific Language for Technical Analysis
- Uses the Anko scripting language (github.com/mattn/anko) as an embedded interpreter
- Defines 100+ technical analysis functions from go-talib (SMA, EMA, RSI, MACD, etc.)
- Provides custom functions: gt, lt, gte, lte, mult, div, and, or, series, cumsum, normalize, lag, shift, sharpe, sign
- `GetColumn()` evaluates DSL expressions on quote data (e.g., "sma(c,20)" calculates 20-period simple moving average of close prices)
- `EvalFilter()` evaluates filter expressions on rows to include/exclude tickers
- Variables in expressions: d (date), o (open), h (high), l (low), c (close), v (volume)

**orderedmap.go**: Thread-safe ordered map implementation
- Maintains insertion order of keys while providing fast lookups
- Used to preserve the order of user-specified columns in output

**parquet.go**: Parquet file generation with Apache Arrow
- Uses Apache Arrow Go library (github.com/apache/arrow/go/v18) for efficient columnar storage
- `inferArrowSchema()` automatically detects column types (timestamp, float64, string) from data
- `convertRowsToArrowTable()` converts [][]string to typed Arrow Table for Parquet writing
- `sortDataByColumns()` physically sorts data before writing for better compression and query performance
- `partitionDataByColumns()` implements Hive-style partitioning (e.g., symbol=AAPL/year=2024/)
- Supports multiple compression codecs: snappy (default), gzip, zstd, none
- Configurable row group size for balancing read granularity vs compression

### Data Flow

1. Parse command-line flags or load YAML config
2. Validate inputs (dates, source, tickers/market)
3. Fetch tickers from market or use provided list
4. For each ticker:
   - Fetch OHLCV data from the specified source
   - Parse user columns (e.g., "sma20=sma(c,20)|rsi2=rsi(c,2)")
   - Evaluate DSL expressions to calculate technical indicators
   - Apply filter expression to last row (if specified)
   - Append ticker data to results if filter passes
5. Drop specified columns
6. Optionally pivot data (rows become columns indexed by date, tickers become column prefixes)
7. Optionally filter to specific target column when pivoting
8. Split data into training/testing sets if split_pct is specified
9. Write output files in requested format(s):
   - CSV: output.csv, output_train.csv, output_test.csv
   - Parquet: output.parquet (or partitioned directories), output_train.parquet, output_test.parquet
   - Both formats can be generated simultaneously

### Key Design Patterns

**DSL Evaluation**: The application uses Anko as an embedded scripting language to evaluate user-defined expressions for columns and filters. This allows users to compose complex technical indicators without code changes.

**Flexible Configuration**: The `StringList` type handles both YAML arrays and pipe/comma-delimited strings, providing flexibility in how users specify tickers and columns.

**Data Transformation Pipeline**: Data flows through a series of transformations (fetch → calculate → filter → pivot → split → write), with each step being optional and configurable.

## Data Sources

- `yahoo`: Yahoo Finance (default)
- `tiingo`: Tiingo API (requires TIINGO_API_TOKEN environment variable or -tiingo-token flag)
- `tiingo-crypto`: Tiingo Crypto API
- `coinbase`: Coinbase

## DSL Expression Syntax

Column expressions use the format: `columnName=expression`
- Use pipe separator (`|`) for multiple columns
- Variables: `d` (date), `o` (open), `h` (high), `l` (low), `c` (close), `v` (volume)
- Example: `sma20=sma(c,20)|rsi14=rsi(c,14)|signal=gt(c,sma(c,50))`

Filter expressions evaluate to boolean on the last row of each ticker:
- Example: `-filter="roc20 > 0 && rsi2 < 30"` (only include tickers where 20-day ROC > 0 and 2-day RSI < 30)

## Configuration Files

YAML configuration files support all command-line flags. Example:

```yaml
start_date: "2020-01-01"
end_date: "2024-12-31"
outfile: "mag7.csv"
columns:
  - roc1=roc(c,1)
  - roc5=roc(c,5)
  - target=shift(roc5,-1)
source: "tiingo"
tickers:
  - AAPL
  - GOOG
  - MSFT
drop_columns: "volume,open,high,low"
target_column: "QQQ_target"
truncate: 50
pivot: true
split_pct: 0.8
```

## Parquet Output

The application supports Parquet file generation in addition to CSV, with advanced features for data organization:

### Output Format Selection

```yaml
# Write only CSV (default if not specified)
output_formats: [csv]

# Write only Parquet
output_formats: [parquet]

# Write both CSV and Parquet
output_formats: [csv, parquet]
```

### Partitioning

Hive-style partitioning creates directory structures for efficient querying:

```yaml
# Partition by symbol: creates symbol=AAPL/, symbol=GOOG/, etc.
parquet_partition_by: [symbol]

# Partition by date (year and month): creates year=2024/month=01/, etc.
parquet_partition_by: [date]
parquet_partition_date_format: "year,month"

# Multi-column partitioning: symbol=AAPL/year=2024/month=01/
parquet_partition_by: [symbol, date]
parquet_partition_date_format: "year,month"
```

Date partition formats:
- `"year"`: year=2024/
- `"year,month"`: year=2024/month=01/
- `"year,month,day"`: year=2024/month=01/day=15/

### Sorting

Physical sorting improves compression and enables predicate pushdown:

```yaml
# Sort by date first, then symbol
parquet_sort_by: [date, symbol]
```

### Compression

```yaml
# Compression codec (default: snappy)
parquet_compression: "snappy"  # Options: snappy, gzip, zstd, none
```

### Row Group Size

```yaml
# Rows per row group (default: 100000)
# Smaller = more granular reads, Larger = better compression
parquet_row_group_size: 50000
```

### Example Configurations

See `parquet_example.yaml` for comprehensive examples of different Parquet configurations.

## Important Notes

- The DSL environment (`e` in dsl.go) is global and stateful. Column definitions persist across evaluations within a ticker.
- Pivoting transforms ticker-date rows into date rows with ticker-prefixed columns (e.g., "close" becomes "AAPL_close", "GOOG_close", etc.)
- When `split_pct` is specified, data is split chronologically (e.g., 0.8 means first 80% training, last 20% testing)
- The `truncate` flag removes the first N rows from each ticker (useful for removing NaN values from indicators with lookback periods)
- Filter expressions use reserved word replacement (e.g., "close" becomes "_close") to avoid conflicts with Anko keywords
- Parquet files use automatic type inference: dates become timestamps, numeric columns become float64, symbols become strings
- When using both CSV and Parquet output with a .csv filename, Parquet files will automatically use .parquet extension
- Partitioned Parquet output creates a directory structure instead of a single file
