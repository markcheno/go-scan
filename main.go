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

type Tickers []string

func (t *Tickers) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single string
	var list []string

	// Try to unmarshal as a single string
	if err := unmarshal(&single); err == nil {
		*t = strings.Split(single, ",")
		return nil
	}

	// Try to unmarshal as a list of strings
	if err := unmarshal(&list); err == nil {
		*t = list
		return nil
	}

	return fmt.Errorf("failed to unmarshal tickers")
}

// Implement the flag.Value interface
func (t *Tickers) String() string {
	return strings.Join(*t, ",")
}

func (t *Tickers) Set(value string) error {
	*t = strings.Split(value, ",")
	return nil
}

type ScanFlags struct {
	ConfigFile  string  `yaml:"-"`
	SaveConfig  bool    `yaml:"-"`
	StartDate   string  `yaml:"start_date"`
	EndDate     string  `yaml:"end_date"`
	Outfile     string  `yaml:"outfile"`
	Columns     string  `yaml:"columns"`
	DropColumns string  `yaml:"drop_columns"`
	Truncate    int     `yaml:"truncate"`
	Source      string  `yaml:"source"`
	Token       string  `yaml:"token"`
	Tickers     Tickers `yaml:"tickers"`
	Market      string  `yaml:"market"`
	SplitPct    float64 `yaml:"split_pct"`
	ListMarkets bool    `yaml:"-"`
	ListTA      bool    `yaml:"-"`
}

var flags ScanFlags

func init() {
	today := time.Now().Format("2006-01-02")
	flag.StringVar(&flags.ConfigFile, "config", "scan.yaml", "yaml config file")
	flag.BoolVar(&flags.SaveConfig, "save", false, "Save the config to a file")
	flag.StringVar(&flags.StartDate, "start", "2024-01-01", "Start date")
	flag.StringVar(&flags.EndDate, "end", today, "End date")
	flag.StringVar(&flags.Outfile, "outfile", "output.csv", "Output CSV file")
	flag.StringVar(&flags.Columns, "columns", "", "sma20=sma(c,20)|rsi2=rsi(c,2) (use --list-ta to see available functions)")
	flag.StringVar(&flags.DropColumns, "drop-columns", "", "Comma-separated list of columns to drop")
	flag.IntVar(&flags.Truncate, "truncate", 0, "Number of rows to truncate from the beginning")
	flag.StringVar(&flags.Source, "source", "yahoo", "Data source (yahoo|tiingo|tiingo-crypto|coinbase)")
	flag.StringVar(&flags.Token, "token", os.Getenv("TIINGO_API_TOKEN"), "tiingo api token")
	flag.Var(&flags.Tickers, "tickers", "Comma-separated list of tickers")
	flag.StringVar(&flags.Market, "market", "", "Market to fetch data from (use --list-markets to see available markets)")
	flag.Float64Var(&flags.SplitPct, "split-pct", 0, "Percentage of data to use for training")
	flag.BoolVar(&flags.ListMarkets, "list-markets", false, "List available markets")
	flag.BoolVar(&flags.ListTA, "list-ta", false, "List available technical analysis functions")
}

// loadConfig loads the configuration from a YAML file
func loadConfig(filename string, config *ScanFlags) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}

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

	encoder := yaml.NewEncoder(file)
	return encoder.Encode(config)
}

func parseUserColumns() (*OrderedMap, error) {
	orderedMap := NewOrderedMap()
	if flags.Columns == "" {
		return orderedMap, nil
	}

	columnList := strings.Split(flags.Columns, "|")

	for _, column := range columnList {
		params := strings.Split(column, "=")
		if len(params) != 2 {
			return nil, fmt.Errorf("invalid column format")
		}
		orderedMap.Set(params[0], params[1])
	}
	return orderedMap, nil
}

func getQuoteColumns(quote quote.Quote, columnMap *OrderedMap) (*OrderedMap, error) {
	orderedMap := NewOrderedMap()
	if columnMap.IsEmpty() {
		return orderedMap, nil
	}
	for _, col := range columnMap.Keys() {
		expr, _ := columnMap.Data()[col].(string)
		result, err := ExprWithQuote(quote, expr)
		if err != nil {
			return nil, err
		}
		orderedMap.Set(col, result)
	}
	return orderedMap, nil
}

func writeToCSV(filename string, allRows [][]string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

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

	flag.Parse()

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
		for _, ta := range GetTaFuncs() {
			fmt.Printf("\t%s\n", ta.Desc)
		}
		return
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
		tickers = flags.Tickers
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
			q, err = quote.NewQuoteFromTiingo(ticker, flags.StartDate, flags.EndDate, flags.Token)
		} else if flags.Source == "tiingo-crypto" {
			q, err = quote.NewQuoteFromTiingoCrypto(ticker, flags.StartDate, flags.EndDate, "d", flags.Token)
		} else if flags.Source == "coinbase" {
			q, err = quote.NewQuoteFromCoinbase(ticker, flags.StartDate, flags.EndDate, "d")
		} else {
			log.Fatalf("Invalid source: %s", flags.Source)
		}

		if err != nil {
			log.Fatalf("Failed to fetch data for %s: %v", ticker, err)
		}
		columns, err := getQuoteColumns(q, columnMap)
		if err != nil {
			log.Fatalf("Failed to add columns for %s: %v", ticker, err)
		}

		// for i := range q.Date {
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
			allRows = append(allRows, record)

			if i < int(float64(len(q.Date))*flags.SplitPct) {
				trainingRows = append(trainingRows, record)
			} else {
				testingRows = append(testingRows, record)
			}
		}
	}

	if flags.DropColumns != "" {
		dropColumns := strings.Split(flags.DropColumns, ",")
		for _, col := range dropColumns {
			for j, header := range allRows[0] {
				if strings.EqualFold(header, col) {
					allRows = dropColumn(allRows, j)
					trainingRows = dropColumn(trainingRows, j)
					testingRows = dropColumn(testingRows, j)
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
