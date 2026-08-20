# Go Scan

Go Scan fetches OHLCV (Open, High, Low, Close, Volume) data for a set of stock tickers,
calculates user-specified technical indicators, filters for conditions, and writes the
results to CSV and/or Parquet files. It runs either as a command-line tool or as a local
web app for building configurations and previewing the data they produce.

## Features

- Fetch OHLCV data from Tiingo, Tiingo Crypto, Coinbase and Binance, at periods from
  1 minute to monthly
- Calculate user-specified technical indicators with a small expression language
- Screen a whole market with a filter expression
- Write results to CSV and/or Parquet, with Hive-style partitioning, physical sorting and
  configurable compression
- Configure from command-line flags, a YAML file, or the web UI
- On-disk quote cache so repeated runs and live previews do not refetch

## Installation

```sh
go install github.com/markcheno/go-scan/cmd/scan@latest
```

That installs a binary named `scan` into your `GOBIN`. The `cmd/scan` suffix matters:
`go install` names the binary after the last element of the path, so installing the module
root would give you `go-scan` instead.

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
| `-source` | `tiingo` | Data source (see `-list-sources`) |
| `-period` | `d` | Bar period; the supported set varies by source |
| `-tiingo-token` | `$TIINGO_API_TOKEN` | Tiingo API token |
| `-tickers` | | Comma or pipe separated symbols |
| `-market` | | Market to expand into symbols (see `-list-markets`) |
| `-columns` | | Pipe separated `name=expression` pairs (see `-list-ta`) |
| `-filter` | | Boolean expression applied to each ticker's last row |
| `-drop-columns` | | Comma separated columns to drop |
| `-lookback` | `auto` | Fetch extra bars before `-start` so indicators are warm: `auto`, `off`, or a bar count |
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
| `-list-sources` | | List data sources and the periods each serves |
| `-list-markets` | | List available markets |
| `-list-ta` | | List available functions |
| `-version` | | Print the version |

Parquet-specific flags: `-parquet-compression`, `-parquet-partition-by`,
`-parquet-partition-date-format`, `-parquet-sort-by`, `-parquet-row-group-size`.

## Sources and periods

`scan -list-sources` prints the sources and what each one serves. The list comes from
go-quote's provider registry, so a provider added there appears here with no change to
go-scan:

```text
binance        1m 3m 5m 15m 30m 1h 2h 4h 6h 8h 12h d 3d w m
coinbase       1m 5m 15m 30m 1h d w
tiingo         d w m
tiingo-crypto  1m 3m 5m 15m 30m 1h 2h 4h 6h 8h 12h d
```

Asking a source for a period it does not serve is rejected before anything is fetched:

```sh
$ scan -source=tiingo -period=3d -tickers=AAPL
error: period: invalid period "3d" for source "tiingo", must be one of: d, w, m
```

`tiingo` and `tiingo-crypto` need an API token; `coinbase` and `binance` do not. Binance
symbols have no separator (`BTCUSDT`), Coinbase uses a dash (`BTC-USD`).

For daily and coarser periods the `date` column holds a bare `2006-01-02` date. For
intraday periods it holds `2006-01-02 15:04:05`, since otherwise every bar in a day would
carry the same value — which would also collide when pivoting on date.

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

## Warm-up

An indicator has no value until it has seen enough bars, and go-talib fills those leading bars
with zeros rather than leaving them empty. Without help, asking for 2024 with an `sma200`
column gives you 199 rows of `0.000000` — not prices, and not something that should reach a
model.

So `-lookback` defaults to `auto`: it works out how much history the column expressions need,
fetches that much extra *before* the start date, and then emits only the rows from the start
date on.

```sh
$ scan -tickers=AAPL -start=2024-01-01 -end=2024-12-31 -columns="sma200=sma(c,200)"
lookback: fetching from 2023-03-04 (200 extra bars) so columns are warm at 2024-01-01
```

The requested range comes back whole, with the indicator already populated on its first row.

The estimate comes from the window sizes in the expressions, and nested windows add up —
`sma(rsi(c,2),200)` needs 202 bars, not 200, because the outer average cannot start until the
inner one is valid. It deliberately over-estimates; surplus bars are discarded. Where it cannot
work the size out — a non-literal period, an unrecognized function — it says so and fetches
what it can:

```text
warning: lookback: cannot derive the warm-up of x=sma(c,someVar) (non-literal period in sma)
```

Pass a bar count instead of `auto` to set it yourself, or `off` for the old behavior of
starting cold. Note that `truncate` still applies on top, dropping N further bars from the
start of the output — with the lookback on you rarely want both.

Because the lookback changes the date range requested from the provider, and the quote cache is
keyed on that range, switching it on or off refetches rather than reusing a cached range.

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
go run ./cmd/scan -serve -dev
```

A few tests need network access and are opt-in:

```sh
GO_SCAN_NETWORK_TESTS=1 go test ./internal/engine -run TestListMarketETF
```

The browser smoke test for the UI is separate and needs Playwright; see
[internal/server/uitest/README.md](internal/server/uitest/README.md).

Layout:

- `cmd/scan` — command-line entry point
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
