package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/markcheno/go-quote"
	"gopkg.in/yaml.v2"
)

// StringList is a custom flag type for a list of strings
type StringList struct {
	items []string
}

// Implement the yaml.Unmarshaler interface
func (sl *StringList) UnmarshalYAML(unmarshal func(any) error) error {
	// Try to unmarshal as a list of strings first
	var items []string
	if err := unmarshal(&items); err == nil {
		sl.items = items
		return nil
	}

	// Try as a single string
	var str string
	if err := unmarshal(&str); err == nil {
		switch {
		case strings.Contains(str, "|"):
			sl.items = strings.Split(str, "|")
		case strings.Contains(str, ","):
			sl.items = strings.Split(str, ",")
		default:
			if str != "" {
				sl.items = []string{str}
			} else {
				sl.items = []string{}
			}
		}
		return nil
	}

	return fmt.Errorf("failed to unmarshal as string or []string")
}

// Add MarshalYAML method to properly serialize the StringList
func (sl StringList) MarshalYAML() (interface{}, error) {
	if len(sl.items) == 0 {
		return []string{}, nil
	}
	return sl.items, nil
}

// Implement the flag.Value interface
func (sl *StringList) String() string {
	return strings.Join(sl.items, "|")
}

func (sl *StringList) Set(value string) error {
	sl.items = strings.Split(value, "|")
	return nil
}

// ScanFlags is a struct to hold the command line flags
type ScanFlags struct {
	TiingoToken             string     `yaml:"-"`
	ListMarkets             bool       `yaml:"-"`
	ListTA                  bool       `yaml:"-"`
	ConfigFile              string     `yaml:"-"`
	Logfile                 string     `yaml:"logfile"`
	Outfile                 string     `yaml:"outfile"`
	StartDate               string     `yaml:"start_date"`
	EndDate                 string     `yaml:"end_date"`
	Filter                  string     `yaml:"filter"`
	Source                  string     `yaml:"source"`
	Tickers                 StringList `yaml:"tickers"`
	Market                  string     `yaml:"market"`
	Columns                 StringList `yaml:"columns"`
	DropColumns             string     `yaml:"drop_columns"`
	TargetColumn            string     `yaml:"target_column"`
	Truncate                int        `yaml:"truncate"`
	Pivot                   bool       `yaml:"pivot"`
	SplitPct                float64    `yaml:"split_pct"`
	OutputFormats           StringList `yaml:"output_formats"`
	ParquetCompression      string     `yaml:"parquet_compression"`
	ParquetPartitionBy      StringList `yaml:"parquet_partition_by"`
	ParquetPartitionDateFmt string     `yaml:"parquet_partition_date_format"`
	ParquetSortBy           StringList `yaml:"parquet_sort_by"`
	ParquetRowGroupSize     int        `yaml:"parquet_row_group_size"`
}

// ScanFlags is a global variable to hold the command line flags
var flags ScanFlags

var (
	Version = "dev"
)

// init initializes the command line flags
func initFlags() {
	today := time.Now().Format("2006-01-02")
	flag.StringVar(&flags.ConfigFile, "config", "", "Configuration file path (load if exists, save if not)")
	flag.StringVar(&flags.TiingoToken, "tiingo-token", os.Getenv("TIINGO_API_TOKEN"), "tiingo api token")
	flag.BoolVar(&flags.ListMarkets, "list-markets", false, "List available markets")
	flag.BoolVar(&flags.ListTA, "list-ta", false, "List available technical analysis functions")
	flag.StringVar(&flags.Logfile, "log", "", "Log file")
	flag.StringVar(&flags.StartDate, "start", "2024-01-01", "Start date")
	flag.StringVar(&flags.EndDate, "end", today, "End date")
	flag.StringVar(&flags.Outfile, "outfile", "output.csv", "Output CSV file")
	flag.Var(&flags.Columns, "columns", "sma20=sma(c,20)|rsi2=rsi(c,2) (Pipe separated columns to add, use --list-ta to see available functions)")
	flag.StringVar(&flags.DropColumns, "drop-columns", "", "Comma-separated list of columns to drop")
	flag.StringVar(&flags.TargetColumn, "target-column", "", "Target column (other target columns will be dropped when pivoting)")
	flag.IntVar(&flags.Truncate, "truncate", 0, "Number of rows to truncate from the beginning")
	flag.BoolVar(&flags.Pivot, "pivot", false, "Pivot the data")
	flag.StringVar(&flags.Filter, "filter", "", "filter expression to apply to last column of each ticker")
	flag.StringVar(&flags.Source, "source", "tiingo", "Data source (yahoo|tiingo|tiingo-crypto|coinbase)")
	flag.Var(&flags.Tickers, "tickers", "Comma-separated list of tickers")
	flag.StringVar(&flags.Market, "market", "", "Market to fetch data from (use --list-markets to see available markets)")
	flag.Float64Var(&flags.SplitPct, "split-pct", 0, "Percentage of data to use for training")
	flag.Var(&flags.OutputFormats, "output-formats", "Output formats (csv|parquet) - pipe separated")
	flag.StringVar(&flags.ParquetCompression, "parquet-compression", "snappy", "Parquet compression codec (snappy|gzip|zstd|none)")
	flag.Var(&flags.ParquetPartitionBy, "parquet-partition-by", "Columns to partition by (pipe separated, e.g. symbol|year)")
	flag.StringVar(&flags.ParquetPartitionDateFmt, "parquet-partition-date-format", "", "Date partition format (year|year,month|year,month,day)")
	flag.Var(&flags.ParquetSortBy, "parquet-sort-by", "Columns to sort by (pipe separated)")
	flag.IntVar(&flags.ParquetRowGroupSize, "parquet-row-group-size", 100000, "Number of rows per row group in Parquet files")
}

// handleConfig handles loading and saving of configuration
func handleConfig(flags *ScanFlags) error {
	if flags.ConfigFile == "" {
		return nil
	}

	// Check if config file exists
	if _, err := os.Stat(flags.ConfigFile); os.IsNotExist(err) {
		// Save current configuration
		log.Printf("Saving configuration to %s", flags.ConfigFile)
		return saveConfig(flags.ConfigFile, flags)
	}

	// Load existing configuration
	log.Printf("Loading configuration from %s", flags.ConfigFile)
	return loadConfig(flags.ConfigFile, flags)
}

// loadConfig loads the configuration from a YAML file
func loadConfig(filename string, config *ScanFlags) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filename, err)
	}
	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to unmarshal data: %v", err)
	}
	return nil
}

// saveConfig saves the configuration to a YAML file
func saveConfig(path string, config *ScanFlags) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", path, err)
	}
	defer file.Close()
	encoder := yaml.NewEncoder(file)
	return encoder.Encode(config)
}

// parseUserColumns parses the user columns and returns an OrderedMap
func parseUserColumns() (*OrderedMap, error) {
	orderedMap := NewOrderedMap()
	if len(flags.Columns.items) == 0 {
		return orderedMap, nil
	}

	columnList := strings.SplitSeq(flags.Columns.String(), "|")

	for column := range columnList {
		params := strings.Split(column, "=")
		if len(params) != 2 {
			return nil, fmt.Errorf("invalid column format")
		}
		orderedMap.Set(params[0], params[1])
	}
	return orderedMap, nil
}

// getColumns returns a map of columns for the specified quote
func getColumns(quote quote.Quote, columnMap *OrderedMap) (*OrderedMap, error) {
	orderedMap := NewOrderedMap()
	if columnMap.IsEmpty() {
		return orderedMap, nil
	}
	for _, column := range columnMap.Keys() {
		expr, _ := columnMap.Data()[column].(string)
		result, err := GetColumn(quote, column, expr)
		if err != nil {
			return nil, err
		}
		orderedMap.Set(column, result)
	}
	return orderedMap, nil
}

// writeToCSV writes the 2D slice to a CSV file
func writeToCSV(filename string, allRows [][]string) error {

	// Get the directory part of the filename
	dir := filepath.Dir(filename)

	// Create the directories if they do not exist
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		log.Fatalf("failed to create directories: %s, %v", filename, err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	log.Printf("Writing to %s", filename)
	for _, record := range allRows {
		err := writer.Write(record)
		if err != nil {
			return err
		}
	}

	return nil
}

// dropColumn removes the column at the specified index from the 2D slice.
func dropColumn(data [][]string, colIndex int) [][]string {
	newData := make([][]string, len(data))
	for i := range data {
		if colIndex < len(data[i]) {
			newRow := make([]string, len(data[i])-1)
			copy(newRow, data[i][:colIndex])
			copy(newRow[colIndex:], data[i][colIndex+1:])
			newData[i] = newRow
		} else {
			newData[i] = data[i]
		}
	}
	return newData
}

// PivotTable transforms a table into a pivoted format
// indexCol specifies which column to use as the index (row labels)
// pivotCol specifies which column values will become new columns
func pivot(input [][]string, indexCol, pivotCol string) [][]string {
	if len(input) < 2 {
		return input
	}

	// Get headers
	headers := input[0]
	if len(headers) < 3 {
		return input
	}

	// Find index and pivot column positions
	indexColPos := -1
	pivotColPos := -1
	for i, h := range headers {
		if h == indexCol {
			indexColPos = i
		}
		if h == pivotCol {
			pivotColPos = i
		}
	}
	if indexColPos == -1 || pivotColPos == -1 {
		return input // Return original if columns not found
	}

	// Create maps to store unique indices and pivot values
	uniqueIndices := make(map[string]bool)
	uniquePivotVals := make(map[string]bool)

	// Store value column names (everything except index and pivot columns)
	valueColumns := make([]string, 0)
	for i, h := range headers {
		if i != indexColPos && i != pivotColPos {
			valueColumns = append(valueColumns, h)
		}
	}

	// Collect unique indices and pivot values
	for _, row := range input[1:] {
		uniqueIndices[row[indexColPos]] = true
		uniquePivotVals[row[pivotColPos]] = true
	}

	// Convert maps to sorted slices
	indices := make([]string, 0, len(uniqueIndices))
	pivotVals := make([]string, 0, len(uniquePivotVals))
	for idx := range uniqueIndices {
		indices = append(indices, idx)
	}
	for pval := range uniquePivotVals {
		pivotVals = append(pivotVals, pval)
	}
	sort.Strings(indices)
	sort.Strings(pivotVals)

	// Create header row for output
	newHeader := make([]string, 1+len(pivotVals)*len(valueColumns))
	newHeader[0] = indexCol
	headerIdx := 1
	for _, pval := range pivotVals {
		for _, valCol := range valueColumns {
			newHeader[headerIdx] = pval + "_" + valCol
			headerIdx++
		}
	}

	// Create map to store values
	// Key format: index_pivotValue_valueColumn
	valueMap := make(map[string]string)
	for _, row := range input[1:] {
		idx := row[indexColPos]
		pval := row[pivotColPos]
		valIdx := 0
		for i, val := range row {
			if i != indexColPos && i != pivotColPos {
				key := idx + "_" + pval + "_" + valueColumns[valIdx]
				valueMap[key] = val
				valIdx++
			}
		}
	}

	// Create output matrix
	result := make([][]string, 1+len(indices))
	result[0] = newHeader

	// Fill in the data
	for i, idx := range indices {
		row := make([]string, 1+len(pivotVals)*len(valueColumns))
		row[0] = idx
		colIdx := 1
		for _, pval := range pivotVals {
			for _, valCol := range valueColumns {
				key := idx + "_" + pval + "_" + valCol
				row[colIdx] = valueMap[key]
				colIdx++
			}
		}
		result[i+1] = row
	}

	return result
}

func main() {
	var err error
	var tickers []string

	// Parse command line flags
	initFlags()
	flag.Parse()

	// Set up logging
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	if flags.Logfile != "" {
		log.Printf("Logging to %s", flags.Logfile)
		//logfile, err := os.OpenFile(flags.Logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		logfile, err := os.OpenFile(flags.Logfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)

		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	// Display help text if no flags are provided
	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(1)
	}

	// Handle configuration
	if err := handleConfig(&flags); err != nil {
		log.Fatalf("Config error: %v", err)
	}

	// Print version information
	if version := flag.Bool("version", false, "Print version information"); *version {
		fmt.Printf("go-scan version %s\n", Version)
		os.Exit(0)
	}

	if flags.ListMarkets {
		fmt.Println("Available markets:")
		for _, market := range quote.ValidMarkets {
			fmt.Println(market)
		}
		return
	}

	if flags.ListTA {
		fmt.Println("Available technical analysis functions:")
		for _, ta := range GetTA() {
			fmt.Printf("\t%s\n", ta.Desc)
		}
		return
	}

	// Validate output file extension if formats are specified
	if len(flags.OutputFormats.items) > 0 {
		hasCSV := false
		hasParquet := false
		for _, format := range flags.OutputFormats.items {
			format = strings.ToLower(strings.TrimSpace(format))
			switch format {
			case "csv":
				hasCSV = true
			case "parquet":
				hasParquet = true
			}
		}

		// If only CSV, require .csv extension
		if hasCSV && !hasParquet && !strings.HasSuffix(flags.Outfile, ".csv") {
			log.Fatalf("Output file must have a .csv extension when only CSV format is requested")
		}
		// If only Parquet, require .parquet extension
		if hasParquet && !hasCSV && !strings.HasSuffix(flags.Outfile, ".parquet") {
			log.Fatalf("Output file must have a .parquet extension when only Parquet format is requested")
		}
	} else {
		// Default to CSV for backward compatibility
		if !strings.HasSuffix(flags.Outfile, ".csv") {
			log.Fatalf("Output file must have a .csv extension")
		}
	}

	if (flags.Source == "tiingo" || flags.Source == "tiingo-crypto") && flags.TiingoToken == "" {
		log.Fatalf("Tiingo token is required")
	}

	// Handle market flag
	if flags.Market != "" {
		if !quote.ValidMarket(flags.Market) {
			log.Fatalf("Invalid market: %s", flags.Market)
		}
		marketTickers, err := quote.NewMarketList(flags.Market)
		if err != nil {
			log.Fatalf("Failed to fetch market data: %v", err)
		}
		tickers = append(tickers, marketTickers...)
	}

	// Handle tickers flag
	if len(flags.Tickers.items) > 0 {
		// Handle comma-separated tickers
		for _, item := range flags.Tickers.items {
			if strings.Contains(item, ",") {
				tickerList := strings.Split(item, ",")
				tickers = append(tickers, tickerList...)
			} else {
				tickers = append(tickers, item)
			}
		}
	}

	// Remove duplicates from tickers slice
	uniqueTickers := make(map[string]bool)
	var finalTickers []string
	for _, ticker := range tickers {
		if !uniqueTickers[ticker] {
			uniqueTickers[ticker] = true
			finalTickers = append(finalTickers, ticker)
		}
	}
	tickers = finalTickers

	if len(tickers) == 0 {
		fmt.Println("No tickers specified")
		flag.Usage()
		os.Exit(1)
	}

	allRows := make([][]string, 0)
	trainingRows := make([][]string, 0)
	testingRows := make([][]string, 0)

	// Parse user columns
	columnMap, err := parseUserColumns()
	if err != nil {
		log.Fatalf("Failed to parse columns: %v", err)
	}

	// Generate headers
	headers := []string{"symbol", "date", "open", "high", "low", "close", "volume"}
	if !columnMap.IsEmpty() {
		headers = append(headers, columnMap.Keys()...)
	}

	// Generate all rows
	allRows = append(allRows, headers)
	trainingRows = append(trainingRows, headers)
	testingRows = append(testingRows, headers)

	for _, ticker := range tickers {
		var q quote.Quote
		var err error
		switch flags.Source {
		case "yahoo":
			q, err = quote.NewQuoteFromYahoo(ticker, flags.StartDate, flags.EndDate, "d", true)
		case "tiingo":
			q, err = quote.NewQuoteFromTiingo(ticker, flags.StartDate, flags.EndDate, flags.TiingoToken)
		case "tiingo-crypto":
			q, err = quote.NewQuoteFromTiingoCrypto(ticker, flags.StartDate, flags.EndDate, "d", flags.TiingoToken)
		case "coinbase":
			q, err = quote.NewQuoteFromCoinbase(ticker, flags.StartDate, flags.EndDate, "d")
		default:
			log.Fatalf("Invalid source: %s", flags.Source)
		}
		if err != nil {
			log.Fatalf("Failed to fetch data for %s: %v", ticker, err)
		}

		columns, err := getColumns(q, columnMap)
		if err != nil {
			log.Fatalf("Failed to add columns for %s: %v", ticker, err)
		}

		var tickerRows [][]string
		for i := flags.Truncate; i < len(q.Date); i++ {
			record := []string{
				q.Symbol,
				q.Date[i].Format("2006-01-02"),
				fmt.Sprintf("%f", q.Open[i]),
				fmt.Sprintf("%f", q.High[i]),
				fmt.Sprintf("%f", q.Low[i]),
				fmt.Sprintf("%f", q.Close[i]),
				fmt.Sprintf("%f", q.Volume[i]),
			}
			for _, col := range columns.Keys() {
				if len(q.Date) != len(columns.Data()[col].([]float64)) {
					log.Fatalf("Length mismatch for %s: %d != %d", col, len(q.Date), len(columns.Data()[col].([]float64)))
				}
				record = append(record, fmt.Sprintf("%f", columns.Data()[col].([]float64)[i]))
			}
			tickerRows = append(tickerRows, record)

			if flags.SplitPct > 0 {
				if i < int(float64(len(q.Date))*flags.SplitPct) {
					trainingRows = append(trainingRows, record)
				} else {
					testingRows = append(testingRows, record)
				}
			}
		}
		addticker, err := EvalFilter(flags.Filter, headers, tickerRows[len(tickerRows)-1])
		if err != nil {
			log.Fatalf("Failed to evaluate filter: %v", err)
		}
		if addticker {
			log.Printf("Saving %s\n", ticker)
			allRows = append(allRows, tickerRows...)
		} else {
			log.Printf("Excluding %s due to filter\n", ticker)
		}
	}

	if flags.DropColumns != "" {
		dropColumns := strings.Split(flags.DropColumns, ",")
		for _, col := range dropColumns {
			for j, header := range allRows[0] {
				if strings.EqualFold(header, col) {
					allRows = dropColumn(allRows, j)
					if flags.SplitPct > 0 {
						trainingRows = dropColumn(trainingRows, j)
						testingRows = dropColumn(testingRows, j)
					}
				}
			}
		}
	}

	if flags.Pivot {
		allRows = pivot(allRows, "date", "symbol")
		if flags.SplitPct > 0 {
			trainingRows = pivot(trainingRows, "date", "symbol")
			testingRows = pivot(testingRows, "date", "symbol")
		}
		//
		if flags.TargetColumn != "" {
			// Drop all target columns except the specified one
			for j, header := range allRows[0] {
				if strings.Contains(header, "target") && flags.TargetColumn != header {
					//fmt.Println("Dropping: ", header)
					allRows = dropColumn(allRows, j)
					if flags.SplitPct > 0 {
						trainingRows = dropColumn(trainingRows, j)
						testingRows = dropColumn(testingRows, j)
					}
				}
			}
		}
	}

	// Determine which formats to write
	shouldWriteCSV := len(flags.OutputFormats.items) == 0 // Default to CSV
	shouldWriteParquet := false

	for _, format := range flags.OutputFormats.items {
		format = strings.ToLower(strings.TrimSpace(format))
		switch format {
		case "csv":
			shouldWriteCSV = true
		case "parquet":
			shouldWriteParquet = true
		}
	}

	// Write CSV files if requested
	if shouldWriteCSV {
		err = writeToCSV(flags.Outfile, allRows)
		if err != nil {
			log.Fatalf("Failed to write CSV: %v", err)
		}

		if flags.SplitPct > 0 {
			trainfile := strings.Replace(flags.Outfile, ".csv", "_train.csv", -1)
			testfile := strings.Replace(flags.Outfile, ".csv", "_test.csv", -1)

			err = writeToCSV(trainfile, trainingRows)
			if err != nil {
				log.Fatalf("Failed to write training CSV: %v", err)
			}

			err = writeToCSV(testfile, testingRows)
			if err != nil {
				log.Fatalf("Failed to write testing CSV: %v", err)
			}
		}
	}

	// Write Parquet files if requested
	if shouldWriteParquet {
		parquetFile := flags.Outfile
		// If the outfile has .csv extension and we're writing Parquet, change it
		if strings.HasSuffix(parquetFile, ".csv") {
			parquetFile = strings.Replace(parquetFile, ".csv", ".parquet", 1)
		}

		err = writeToParquet(parquetFile, allRows, &flags)
		if err != nil {
			log.Fatalf("Failed to write Parquet: %v", err)
		}

		if flags.SplitPct > 0 {
			trainfile := strings.Replace(parquetFile, ".parquet", "_train.parquet", -1)
			testfile := strings.Replace(parquetFile, ".parquet", "_test.parquet", -1)

			err = writeToParquet(trainfile, trainingRows, &flags)
			if err != nil {
				log.Fatalf("Failed to write training Parquet: %v", err)
			}

			err = writeToParquet(testfile, testingRows, &flags)
			if err != nil {
				log.Fatalf("Failed to write testing Parquet: %v", err)
			}
		}
	}
}
