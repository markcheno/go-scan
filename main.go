package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/markcheno/go-quote"
	"gopkg.in/yaml.v2"
)

// StringList is a custom flag type for a list of strings
type StringList []string

// Implement the yaml.Unmarshaler interface
func (sl *StringList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single string
	var list []string

	// Try to unmarshal as a single string
	if err := unmarshal(&single); err == nil {
		if strings.Contains(single, "|") {
			*sl = strings.Split(single, "|")
		} else {
			*sl = strings.Split(single, ",")
		}
		return nil
	}

	// Try to unmarshal as a list of strings
	if err := unmarshal(&list); err == nil {
		*sl = list
		return nil
	}

	return nil
}

// Implement the flag.Value interface
func (sl *StringList) String() string {
	return strings.Join(*sl, "|")
}

func (sl *StringList) Set(value string) error {
	*sl = strings.Split(value, "|")
	return nil
}

// ScanFlags is a struct to hold the command line flags
type ScanFlags struct {
	ConfigFile  string     `yaml:"-"`
	SaveConfig  bool       `yaml:"-"`
	TiingoToken string     `yaml:"-"`
	ListMarkets bool       `yaml:"-"`
	ListTA      bool       `yaml:"-"`
	Logfile     string     `yaml:"logfile"`
	Outfile     string     `yaml:"outfile"`
	StartDate   string     `yaml:"start_date"`
	EndDate     string     `yaml:"end_date"`
	Filter      string     `yaml:"filter"`
	Source      string     `yaml:"source"`
	Tickers     StringList `yaml:"tickers"`
	Market      string     `yaml:"market"`
	Columns     StringList `yaml:"columns"`
	DropColumns string     `yaml:"drop_columns"`
	Truncate    int        `yaml:"truncate"`
	SplitPct    float64    `yaml:"split_pct"`
}

// ScanFlags is a global variable to hold the command line flags
var flags ScanFlags

// init initializes the command line flags
func init() {
	today := time.Now().Format("2006-01-02")
	flag.StringVar(&flags.ConfigFile, "config", "scan.yaml", "Yaml config file")
	flag.BoolVar(&flags.SaveConfig, "save", false, "Save the config to a file")
	flag.StringVar(&flags.TiingoToken, "tiingo-token", os.Getenv("TIINGO_API_TOKEN"), "tiingo api token")
	flag.BoolVar(&flags.ListMarkets, "list-markets", false, "List available markets")
	flag.BoolVar(&flags.ListTA, "list-ta", false, "List available technical analysis functions")
	flag.StringVar(&flags.Logfile, "log", "", "Log file")
	flag.StringVar(&flags.StartDate, "start", "2024-01-01", "Start date")
	flag.StringVar(&flags.EndDate, "end", today, "End date")
	flag.StringVar(&flags.Outfile, "outfile", "output.csv", "Output CSV file")
	flag.Var(&flags.Columns, "columns", "sma20=sma(c,20)|rsi2=rsi(c,2) (Pipe separated columns to add, use --list-ta to see available functions)")
	flag.StringVar(&flags.DropColumns, "drop-columns", "", "Comma-separated list of columns to drop")
	flag.IntVar(&flags.Truncate, "truncate", 0, "Number of rows to truncate from the beginning")
	flag.StringVar(&flags.Filter, "filter", "", "filter expression to apply to last column of each ticker")
	flag.StringVar(&flags.Source, "source", "yahoo", "Data source (yahoo|tiingo|tiingo-crypto|coinbase)")
	flag.Var(&flags.Tickers, "tickers", "Comma-separated list of tickers")
	flag.StringVar(&flags.Market, "market", "", "Market to fetch data from (use --list-markets to see available markets)")
	flag.Float64Var(&flags.SplitPct, "split-pct", 0, "Percentage of data to use for training")
}

// loadConfig loads the configuration from a YAML file
func loadConfig(filename string, config *ScanFlags) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	log.Println("Loading config from: ", filename)
	if err := yaml.Unmarshal(data, config); err != nil {
		log.Fatalf("failed to unmarshal data: %v", err)
	}

}

// saveConfig saves the configuration to a YAML file
func saveConfig(path string, config *ScanFlags) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	log.Println("Saving config to: ", path)
	encoder := yaml.NewEncoder(file)
	return encoder.Encode(config)
}

// parseUserColumns parses the user columns and returns an OrderedMap
func parseUserColumns() (*OrderedMap, error) {
	orderedMap := NewOrderedMap()
	if len(flags.Columns) == 0 {
		return orderedMap, nil
	}

	columnList := strings.Split(flags.Columns.String(), "|")

	for _, column := range columnList {
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
		result, err := GetColumn(quote, expr)
		if err != nil {
			return nil, err
		}
		orderedMap.Set(column, result)
	}
	return orderedMap, nil
}

// writeToCSV writes the 2D slice to a CSV file
func writeToCSV(filename string, allRows [][]string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	log.Println("Writing to: ", filename)
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

func main() {
	var err error
	var tickers []string

	// Parse command line flags
	flag.Parse()

	// Set up logging
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Logfile: %s", flags.Logfile)
	if flags.Logfile != "" {
		logfile, err := os.OpenFile(flags.Logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
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

	loadConfig(flags.ConfigFile, &flags)

	if flags.SaveConfig {
		err = saveConfig(flags.ConfigFile, &flags)
		if err != nil {
			log.Fatalf("Failed to save config: %v", err)
		}
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

	if (flags.Source == "tiingo" || flags.Source == "tiingo-crypto") && flags.TiingoToken == "" {
		log.Fatalf("Tiingo token is required")
	}

	if flags.Market != "" {
		if !quote.ValidMarket(flags.Market) {
			log.Fatalf("Invalid market: %s", flags.Market)
		}
		tickers, err = quote.NewMarketList(flags.Market)
		if err != nil {
			log.Fatalf("Failed to fetch market data: %v", err)
		}
	}

	if len(flags.Tickers) > 0 {
		tickers = append(tickers, flags.Tickers...)
	}

	if !strings.HasSuffix(flags.Outfile, ".csv") {
		log.Fatalf("Output file must have a .csv extension")
	}

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
		if flags.Source == "yahoo" {
			q, err = quote.NewQuoteFromYahoo(ticker, flags.StartDate, flags.EndDate, "d", true)
		} else if flags.Source == "tiingo" {
			q, err = quote.NewQuoteFromTiingo(ticker, flags.StartDate, flags.EndDate, flags.TiingoToken)
		} else if flags.Source == "tiingo-crypto" {
			q, err = quote.NewQuoteFromTiingoCrypto(ticker, flags.StartDate, flags.EndDate, "d", flags.TiingoToken)
		} else if flags.Source == "coinbase" {
			q, err = quote.NewQuoteFromCoinbase(ticker, flags.StartDate, flags.EndDate, "d")
		} else {
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
			log.Printf("Saving: %s\n", ticker)
			allRows = append(allRows, tickerRows...)
		} else {
			log.Printf("Excluding: %s\n", ticker)
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
