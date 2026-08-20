# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

go-scan fetches OHLCV (Open, High, Low, Close, Volume) data for stock tickers, calculates technical indicators through a small expression language, filters tickers, and writes results to CSV and/or Parquet. It runs as a CLI or as a local web app (`scan -serve`) for building configs and previewing their output.

## Build and Test Commands

```bash
go build -o scan ./cmd/scan # build
go test ./... -race         # run all tests
go test ./internal/engine -run TestName -v
go mod tidy
```

Tests never hit the network by default; the fetcher is an interface and the engine tests
use a stub. The one exception is gated:

```bash
GO_SCAN_NETWORK_TESTS=1 go test ./internal/engine -run TestListMarketETF
```

The browser smoke test for the web UI is separate and needs Playwright; see
`internal/server/uitest/README.md`.

## Running

```bash
scan -serve -open                     # web UI on 127.0.0.1:8080
go run ./cmd/scan -serve -dev         # web assets from disk, run from the repo root
scan -config=config.yaml              # run from a config file
scan -tickers=AAPL,MSFT -start=2024-01-01 -columns="sma20=sma(c,20)"
scan -list-sources
scan -list-markets
scan -list-ta
```

## Architecture

Three packages. `cmd/scan` is a thin CLI shell; everything real lives in `internal/engine`,
and `internal/server` is an HTTP layer over it. Both entry points share one code path, so
the web UI cannot drift from what the CLI does.

### internal/engine

**config.go** — `Config` (the whole option surface, 21 persisted YAML fields), `StringList`
(accepts a YAML sequence or a pipe/comma delimited scalar; always marshals back as a
sequence), and `LoadConfig`/`SaveConfig`/`MarshalConfig`/`HandleConfig`. `MarshalConfig`
uses the same encoder as `SaveConfig` so a rendered preview is byte-identical to the file.

`Sources()`, `Periods(source)` and `Markets()` all read go-quote's registries rather than
hardcoding anything, so a provider or market added upstream needs no change here. The one
thing the registry does not describe is credentials, so which sources need a token is still
knowledge go-scan holds itself, in `validateUniverse`.

**validate.go** — `Validate(cfg) []FieldError`. Every problem carries the field name (and
list index) it belongs to, plus an error/warning severity, so the CLI and the web UI report
the same things and the UI can attach a message to the right input. `ProjectedHeaders`
predicts the output columns, which validation uses to reject sort/partition columns that
will not exist.

**catalog.go** — the single source of truth for every symbol the expression language
exposes: name, args, doc, category, and whether it is a callable function or one of the
nine `MaType` constants. `-list-ta` output and the UI's function reference are both derived
from it, so signatures cannot drift from the implementation.

**dsl.go** — the Anko interpreter. A base env holds the catalog and is never mutated after
init; `NewEvaluator()` returns a child scope. **One Evaluator per ticker**: columns defined
by earlier expressions are visible to later ones, and nothing leaks to another ticker or
another HTTP request. Evaluation runs under a context timeout and recovers panics from
go-talib and the vector helpers, turning them into errors attributed to the column. An
Evaluator is not safe for concurrent use.

`EvalFilter` binds row values as float64 where they parse, so filter comparisons are
numeric. Reserved identifiers (`open`, `high`, `low`, `close`, `date`) are prefixed with an
underscore using a word-boundary regexp, so a column named `close_ma` or `AAPL_close` is
left alone.

**engine.go** — `Run(ctx, cfg, opts) (*Result, error)`, the whole pipeline. Options control
concurrency, progress and log callbacks, `DryRun` (compute but write nothing), `MaxTickers`
and `MaxBars` — the last three are what make the UI's live preview cheap without a second
code path. Per-ticker failures land in `Result.Errors` and the run continues.

**fetch.go** — source dispatch, bounded-parallel fetching, and a JSON file cache under the
user cache dir keyed by source/ticker/date range/period. Ranges ending today expire after an hour;
historical ranges never do. `Fetcher` and `MarketLister` are interfaces so tests run without
network access.

**transform.go / csv.go / parquet.go** — pivot, column dropping, and the writers.

### internal/server

`server.go` (routing and loopback hardening), `api.go` (handlers), `jobs.go` (background
runs and their SSE event streams), `assets.go` (`go:embed` of `web/`).

The server binds loopback only and refuses anything else, rejects non-loopback `Host`
headers and cross-origin requests, and requires a per-process token that is injected into
`index.html` in place of `__SCAN_TOKEN__`. It is unauthenticated and writes files anywhere
the user can, so it must never be exposed off the machine.

Endpoints: `/api/meta`, `/api/validate`, `/api/preview`, `/api/scan`, `/api/run`,
`/api/jobs/{id}/events` (SSE), `/api/jobs/{id}/cancel`, `/api/config` (GET/PUT),
`/api/config/yaml`, `/api/files`, `/api/cache/clear`.

### internal/server/web

Vanilla ES modules, no build step: `index.html`, `app.css`, `app.js` (state and wiring),
`api.js`, `grid.js` (sortable, windowed table), `funcs.js` (function reference and
autocomplete). Served embedded, or from disk with `-dev`.

Two things to preserve when editing `app.js`:

- Column rows are **not** rebuilt on every keystroke — validation writes error text into
  existing rows — because rebuilding steals focus mid-typing. Anything derived from the
  column names must be refreshed in `refreshColumnMeta()`, which `onChange` calls.
- `readConfig()` must keep emitting exactly the JSON field names in `engine.Config`. The
  server decodes with `DisallowUnknownFields`, so a rename on either side breaks every
  request.

## Data flow

1. Validate the config; blocking problems stop the run.
2. Resolve the market and ticker list into a deduplicated universe.
3. Fetch every ticker with bounded concurrency, through the cache. Failures are recorded, not fatal.
4. Per ticker: evaluate the column expressions in a fresh Evaluator, format rows as strings,
   evaluate the filter against the last row, and keep the ticker only if it passes.
5. Drop columns, then pivot if requested (and prune other target columns).
6. Split each ticker's own history into train/test when `split_pct` is set.
7. Write CSV and/or Parquet.

## Data sources

- `tiingo` — requires `TIINGO_API_TOKEN` or `-tiingo-token`
- `tiingo-crypto` — same token
- `coinbase` — no token
- `binance` — no token

The set is not hardcoded; it is `quote.ProviderNames()`. Each provider advertises the periods
it serves and `Validate` rejects a mismatch up front, so `-source=tiingo -period=3d` fails
before any request. Yahoo was removed upstream: its endpoint stopped returning quote data.

## Expression syntax

Columns are `name=expression`, pipe separated on the command line. Variables: `d` (date),
`o`, `h`, `l`, `c`, `v`. Later columns may reference earlier ones by name:

```text
sma20=sma(c,20)|above=gt(c,sma20)|target=shift(roc(c,5),-1)
```

Filters are boolean expressions over the output column names, evaluated against each
ticker's last row: `-filter="close > 100 && rsi2 < 30"`.

## Things worth knowing

- Values are formatted with `%f` (six decimals) everywhere in the table, including volume.
- `truncate` drops the first N bars per ticker; it exists to cut the warm-up period where
  indicators are still zero.
- Pivoting turns ticker-date rows into date rows with ticker-prefixed columns
  (`close` becomes `AAPL_close`).
- `sharpe` divides by the variance rather than the standard deviation and is not
  annualized, so it is not really a Sharpe ratio. Kept as-is for compatibility.
- `-market=etf` is the one market not served by an HTTP JSON API: go-quote fetches the
  ~4400-symbol NASDAQ directory over anonymous FTP. It is handled inside go-quote now.
- The `date` column is a bare date for daily and coarser periods and a
  `2006-01-02 15:04:05` timestamp for intraday ones — see `DateColumnLayout`. Without that,
  every intraday bar in a day would share one date value and pivoting on date would collide.
- Market membership is cached for 24 hours; quote ranges ending today expire after an hour
  and historical ranges never do. That comparison is by calendar date, not instant, so it
  stays correct in timezones whose local date differs from UTC's.
- Parquet schema is inferred from the first 1000 rows after sorting.
- With both CSV and Parquet output and a `.csv` outfile, the Parquet files get `.parquet`.
- Partitioned Parquet output creates a directory tree instead of a single file.
