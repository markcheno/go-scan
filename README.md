# Go Scan

Go Scan is a Go application designed to fetch OHLCV (Open, High, Low, Close, Volume) data for a series of stock tickers, calculate various user-specified technical indicators, filter for conditions and write the results to CSV files. The application supports multiple data sources and allows for flexible configuration through command-line flags and YAML configuration files.

## Features

- Fetch OHLCV data from multiple data sources (e.g., Yahoo, Tiingo)
- Calculate user-specified technical indicators
- Write results to CSV files
- Support for both command-line flags and YAML configuration files
- List available markets and technical analysis functions

## Installation

```sh
go install github.com/markcheno/go-scan
```

## Usage

### Command-Line Flags

You can configure the application using command-line flags. Here are some examples:

```sh
scan -tickers=AAPL,GOOG,MSFT -start=2024-01-01 -end=2024-12-31 -outfile=output.csv -columns="sma20=sma(c,20)|rsi2=rsi(c,2)"
```

### YAML Configuration

You can also configure the application using a YAML file. Here is an example `config.yaml`:

```yaml
start_date: "2020-01-01"
end_date: "2024-12-31"
outfile: "mag7.csv"
columns: "sma20=sma(c,20)|sma50=sma(c,50)|sma200=sma(c,200)|roc20=roc(c,20)|roc50=roc(c,50)|roc200=roc(c,200)"
source: "yahoo"
tickers: "AAPL,GOOG,MSFT,NVDA,AMZN,META,NFLX"
split_pct: 0.8
```

Run the application with the YAML configuration file:

```sh
scan -config=config.yaml
```

### Available Flags

- `-config`: YAML config file (default "signal.yaml")
- `-save`: Save the config to a file
- `-start`: Start date (default "2024-01-01")
- `-end`: End date (default today)
- `-outfile`: Output CSV file (default "output.csv")
- `-columns`: Columns to calculate (e.g., "sma20=sma(c,20)|rsi2=rsi(c,2)")
- `-source`: Data source (e.g., "yahoo", "tiingo", "tiingo-crypto", "coinbase")
- `-token`: API token for data source
- `-tickers`: Comma-separated list of tickers
- `-market`: Market to fetch data from (use `--list-markets` to see available markets)
- `-split-pct`: Percentage of data to use for training
- `-list-markets`: List available markets
- `-list-ta`: List available technical analysis functions

### Example

```sh
scan -tickers=AAPL,GOOG,MSFT -start=2024-01-01 -end=2024-12-31 -outfile=output.csv -columns="sma20=sma(c,20)|rsi2=rsi(c,2)"
```

## Development

1. Clone the repository:

   ```sh
   git clone https://github.com/yourusername/go-scan.git
   cd go-scan
   ```

2. Install dependencies:

   ```sh
   go mod tidy
   ```

### Contributing

Contributions are welcome! Please open an issue or submit a pull request.

### License

This project is licensed under the MIT License. See the LICENSE file for details.

### Acknowledgements

- [go-quote](https://github.com/markcheno/go-quote)
- [go-talib](https://github.com/markcheno/go-talib)
- [anko](https://github.com/mattn/anko)
