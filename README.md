# Go Scan

Go Scan fetches OHLCV (Open, High, Low, Close, Volume) data for a set of stock tickers,
calculates user-specified technical indicators, filters for conditions, and writes the
results to CSV and/or Parquet files. It runs either as a command-line tool or as a local
web app for building configurations and previewing the data they produce.

## Features

- Fetch OHLCV data from Tiingo, Tiingo Crypto and Coinbase
- Calculate user-specified technical indicators with a small expression language
- Screen a whole market with a filter expression
- Write results to CSV and/or Parquet, with Hive-style partitioning, physical sorting and
  configurable compression
- Configure from command-line flags, a YAML file, or the web UI
- On-disk quote cache so repeated runs and live previews do not refetch

## Installation

```sh
go install github.com/markcheno/go-scan@latest
```

## The web UI

```sh
scan -serve -open
```

This starts a local single-page app at `http://127.0.0.1:8080`:

- **Config form** covering every option, with live validation attached to the field at fault
- **Data** tab previewing the actual output rows for a few sampled tickers
- **Scan** tab running the filter across the whole universe and listing which tickers passed,
  with the values that decided it
- **YAML** tab showing exactly what will be written to disk, rendered by the same encoder
  that writes it
- **Run** tab executing the full scan with streaming progress and download links

The server binds loopback only, refuses cross-origin requests and requires a per-process
token, because it is unauthenticated and writes files anywhere you can.

| Flag | Default | Purpose |
| --- | --- | --- |
| `-serve` | off | Start the web UI |
| `-addr` | `127.0.0.1:8080` | Listen address; must be loopback |
| `-open` | off | Open a browser once listening |
| `-dev` | off | Serve web assets from disk instead of the embedded copy; must run from the repo root |

## Command-line usage

```sh
scan -tickers=AAPL,GOOG,MSFT -start=2024-01-01 -end=2024-12-31 \
     -outfile=output.csv -columns="sma20=sma(c,20)|rsi2=rsi(c,2)"
```

### YAML configuration

```yaml
start_date: "2020-01-01"
end_date: "2024-12-31"
outfile: "mag7.csv"
source: "tiingo"
tickers:
  - AAPL
  - GOOG
  - MSFT
columns:
  - sma20=sma(c,20)
  - roc20=roc(c,20)
filter: "close > sma20"
split_pct: 0.8
```

`-config` loads the file if it exists and writes the current flags to it if it does not:

```sh
scan -config=config.yaml
```

### Flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `-config` | | YAML config file: load if present, save if not |
| `-start` | `2024-01-01` | Start date, `YYYY-MM-DD` |
| `-end` | today | End date, `YYYY-MM-DD` |
| `-source` | `tiingo` | `tiingo`, `tiingo-crypto` or `coinbase` |
| `-tiingo-token` | `$TIINGO_API_TOKEN` | Tiingo API token |
| `-tickers` | | Comma or pipe separated symbols |
| `-market` | | Market to expand into symbols (see `-list-markets`) |
| `-columns` | | Pipe separated `name=expression` pairs (see `-list-ta`) |
| `-filter` | | Boolean expression applied to each ticker's last row |
| `-drop-columns` | | Comma separated columns to drop |
| `-truncate` | `0` | Drop the first N bars of each ticker |
| `-pivot` | off | One row per date, columns prefixed by ticker |
| `-target-column` | | With `-pivot`, keep only this target column |
| `-split-pct` | `0` | Fraction of each ticker's history used for training |
| `-outfile` | `output.csv` | Output path |
| `-output-formats` | `csv` | `csv`, `parquet` or both, pipe separated |
| `-log` | stdout | Log file |
| `-concurrency` | `6` | Tickers fetched at once |
| `-cache-dir` | user cache dir | Quote cache location |
| `-no-cache` | off | Bypass the quote cache |
| `-list-markets` | | List available markets |
| `-list-ta` | | List available functions |
| `-version` | | Print the version |

Parquet-specific flags: `-parquet-compression`, `-parquet-partition-by`,
`-parquet-partition-date-format`, `-parquet-sort-by`, `-parquet-row-group-size`.

## Expressions

Columns are written as `name=expression`. Available variables are `d` (date), `o`, `h`,
`l`, `c` and `v`. Later columns may reference earlier ones by name:

```text
sma20=sma(c,20)|above=gt(c,sma20)|target=shift(roc(c,5),-1)
```

Filters are boolean expressions evaluated against each ticker's last row, using the output
column names. Values are compared numerically:

```sh
-filter="close > 100 && rsi2 < 30"
```

Run `scan -list-ta` for the full list — 110 functions plus the 9 moving-average type
constants that `matype` arguments take — or open the function reference in the web UI.

## Parquet output

```yaml
output_formats: [parquet]
parquet_compression: "snappy"   # snappy | gzip | zstd | none
parquet_partition_by: [symbol]  # symbol=AAPL/ directories
parquet_sort_by: [date, symbol] # physical sort for compression and pushdown
parquet_row_group_size: 100000
```

Partitioning by `date` also accepts `parquet_partition_date_format` of `year`,
`year,month` or `year,month,day`.

## Development

```sh
go build ./...
go test ./... -race

# Edit the UI without rebuilding. -dev reads internal/server/web from the
# working directory, so run it from the repo root.
go run . -serve -dev
```

A few tests need network access and are opt-in:

```sh
GO_SCAN_NETWORK_TESTS=1 go test ./internal/engine -run TestListMarketETF
```

The browser smoke test for the UI is separate and needs Playwright; see
[internal/server/uitest/README.md](internal/server/uitest/README.md).

Layout:

- `main.go` — command-line entry point
- `internal/engine` — config, validation, the expression language, the scan pipeline and writers
- `internal/server` — HTTP API and the embedded web UI under `internal/server/web`

### Contributing

Contributions are welcome. Please open an issue or submit a pull request.

### License

MIT. See the LICENCE file.

### Acknowledgements

- [go-quote](https://github.com/markcheno/go-quote)
- [go-talib](https://github.com/markcheno/go-talib)
- [anko](https://github.com/mattn/anko)
