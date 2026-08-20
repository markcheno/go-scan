package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/markcheno/go-scan/internal/engine"
	"github.com/markcheno/go-scan/internal/server"
)

// Version is set at build time with -ldflags "-X main.Version=...".
var Version = "dev"

// cliFlags holds the options that are not part of the persisted config.
type cliFlags struct {
	config      string
	listSources bool
	listMarkets bool
	listTA      bool
	version     bool
	serve       bool
	addr        string
	open        bool
	dev         bool
	cacheDir    string
	noCache     bool
	concurrency int
}

func main() {
	cfg := engine.DefaultConfig()
	var cli cliFlags

	flag.StringVar(&cli.config, "config", "", "Configuration file path (load if it exists, save if it does not)")
	flag.BoolVar(&cli.listSources, "list-sources", false, "List available data sources and the periods each one serves")
	flag.BoolVar(&cli.listMarkets, "list-markets", false, "List available markets")
	flag.BoolVar(&cli.listTA, "list-ta", false, "List available technical analysis functions")
	flag.BoolVar(&cli.version, "version", false, "Print version information")
	flag.BoolVar(&cli.serve, "serve", false, "Start the web UI")
	flag.StringVar(&cli.addr, "addr", "127.0.0.1:8080", "Address for -serve to listen on")
	flag.BoolVar(&cli.open, "open", false, "Open a browser when -serve starts")
	flag.BoolVar(&cli.dev, "dev", false, "Serve web assets from disk instead of the embedded copy")
	flag.StringVar(&cli.cacheDir, "cache-dir", engine.DefaultCacheDir(), "Directory for the quote cache")
	flag.BoolVar(&cli.noCache, "no-cache", false, "Bypass the quote cache")
	flag.IntVar(&cli.concurrency, "concurrency", engine.DefaultConcurrency, "Number of tickers to fetch at once")

	flag.StringVar(&cfg.TiingoToken, "tiingo-token", cfg.TiingoToken, "Tiingo API token")
	flag.StringVar(&cfg.Logfile, "log", cfg.Logfile, "Log file")
	flag.StringVar(&cfg.StartDate, "start", cfg.StartDate, "Start date")
	flag.StringVar(&cfg.EndDate, "end", cfg.EndDate, "End date")
	flag.StringVar(&cfg.Outfile, "outfile", cfg.Outfile, "Output file")
	flag.Var(&cfg.Columns, "columns", `Pipe separated columns to add, e.g. "sma20=sma(c,20)|rsi2=rsi(c,2)" (see -list-ta)`)
	flag.StringVar(&cfg.DropColumns, "drop-columns", cfg.DropColumns, "Comma-separated list of columns to drop")
	flag.StringVar(&cfg.TargetColumn, "target-column", cfg.TargetColumn, "Target column (other target columns are dropped when pivoting)")
	flag.IntVar(&cfg.Truncate, "truncate", cfg.Truncate, "Number of rows to truncate from the beginning of each ticker")
	flag.StringVar(&cfg.Lookback, "lookback", cfg.Lookback, `Extra bars to fetch before -start so indicators are warm: "auto", "off", or a bar count`)
	flag.BoolVar(&cfg.Pivot, "pivot", cfg.Pivot, "Pivot the data")
	flag.StringVar(&cfg.Filter, "filter", cfg.Filter, "Filter expression applied to the last row of each ticker")
	flag.StringVar(&cfg.Source, "source", cfg.Source, "Data source ("+strings.Join(engine.Sources(), "|")+")")
	flag.StringVar(&cfg.Period, "period", cfg.Period, "Bar period; supported values vary by source (see -list-sources)")
	flag.Var(&cfg.Tickers, "tickers", "Comma or pipe separated list of tickers")
	flag.StringVar(&cfg.Market, "market", cfg.Market, "Market to fetch tickers from (see -list-markets)")
	flag.Float64Var(&cfg.SplitPct, "split-pct", cfg.SplitPct, "Fraction of data to use for training")
	flag.Var(&cfg.OutputFormats, "output-formats", "Output formats (csv|parquet), pipe separated")
	flag.StringVar(&cfg.ParquetCompression, "parquet-compression", cfg.ParquetCompression, "Parquet compression codec ("+strings.Join(engine.Compressions, "|")+")")
	flag.Var(&cfg.ParquetPartitionBy, "parquet-partition-by", "Columns to partition by, pipe separated (e.g. symbol|date)")
	flag.StringVar(&cfg.ParquetPartitionDateFmt, "parquet-partition-date-format", cfg.ParquetPartitionDateFmt, "Date partition format ("+strings.Join(engine.PartitionDateFormats, "|")+")")
	flag.Var(&cfg.ParquetSortBy, "parquet-sort-by", "Columns to sort by, pipe separated")
	flag.IntVar(&cfg.ParquetRowGroupSize, "parquet-row-group-size", cfg.ParquetRowGroupSize, "Rows per row group in Parquet files")

	flag.Parse()

	if cli.version {
		fmt.Printf("go-scan version %s\n", Version)
		return
	}
	if cli.listSources {
		fmt.Println("Available data sources:")
		for _, source := range engine.Sources() {
			fmt.Printf("\t%-14s %s\n", source, strings.Join(engine.Periods(source), " "))
		}
		return
	}
	if cli.listMarkets {
		fmt.Println("Available markets:")
		for _, market := range engine.Markets() {
			fmt.Printf("\t%s\n", market)
		}
		return
	}
	if cli.listTA {
		fmt.Println("Available technical analysis functions:")
		for _, f := range engine.Catalog() {
			fmt.Printf("\t%s\n", f.Desc())
		}
		return
	}

	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(1)
	}

	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime)

	// Check for the file before HandleConfig, which creates it when missing.
	configExisted := false
	if cli.config != "" {
		_, err := os.Stat(cli.config)
		configExisted = err == nil
	}
	if err := engine.HandleConfig(cli.config, &cfg); err != nil {
		log.Fatalf("Config error: %v", err)
	}
	if cli.config != "" {
		if configExisted {
			log.Printf("Loaded configuration from %s", cli.config)
		} else {
			log.Printf("Saved configuration to %s", cli.config)
		}
	}

	if cfg.Logfile != "" {
		logfile, err := os.OpenFile(cfg.Logfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cache := openCache(cli)

	if cli.serve {
		srv := &server.Server{
			Addr:        cli.addr,
			Cache:       cache,
			BaseConfig:  cfg,
			DevAssets:   cli.dev,
			OpenBrowser: cli.open,
			Concurrency: cli.concurrency,
		}
		if err := srv.ListenAndServe(ctx); err != nil {
			log.Fatalf("Server error: %v", err)
		}
		return
	}

	problems := engine.Validate(&cfg)
	for _, p := range problems {
		if p.Severity == engine.SeverityWarning {
			log.Printf("warning: %s", p.Error())
		}
	}
	if engine.HasErrors(problems) {
		for _, p := range problems {
			if p.Severity == engine.SeverityError {
				log.Printf("error: %s", p.Error())
			}
		}
		os.Exit(1)
	}

	result, err := engine.Run(ctx, &cfg, engine.Options{
		Fetcher:     engine.NewQuoteFetcher(cache),
		Concurrency: cli.concurrency,
		Log:         func(msg string) { log.Print(msg) },
	})
	if err != nil {
		log.Fatalf("Scan failed: %v", err)
	}
	for _, e := range result.Errors {
		log.Printf("warning: %s: %s", e.Ticker, e.Message)
	}
	kept := 0
	for _, v := range result.Verdicts {
		if v.Passed {
			kept++
		}
	}
	log.Printf("Done: %d rows from %d of %d tickers in %s",
		len(result.Rows), kept, len(result.Tickers), result.Elapsed.Round(time.Millisecond))
}

// openCache prepares the quote cache, degrading to no cache on failure rather
// than refusing to run.
func openCache(cli cliFlags) *engine.Cache {
	if cli.noCache {
		return nil
	}
	cache, err := engine.NewCache(cli.cacheDir)
	if err != nil {
		log.Printf("warning: quote cache disabled: %v", err)
		return nil
	}
	return cache
}
