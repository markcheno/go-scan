package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/markcheno/go-quote"
	"gopkg.in/yaml.v2"
)

// StringList holds a list of strings that can be expressed in YAML either as a
// sequence or as a single pipe/comma delimited scalar. It also satisfies
// flag.Value so the same type backs the command line flags.
type StringList struct {
	items []string
}

// NewStringList builds a StringList from a slice.
func NewStringList(items ...string) StringList {
	return StringList{items: items}
}

// Items returns the underlying strings.
func (sl StringList) Items() []string { return sl.items }

// Len returns the number of items.
func (sl StringList) Len() int { return len(sl.items) }

// UnmarshalYAML accepts either a sequence or a delimited scalar.
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
		sl.items = splitDelimited(str)
		return nil
	}

	return fmt.Errorf("failed to unmarshal as string or []string")
}

// MarshalYAML always emits a sequence, so a saved config round-trips
// unambiguously regardless of how it was originally written.
func (sl StringList) MarshalYAML() (any, error) {
	if len(sl.items) == 0 {
		return []string{}, nil
	}
	return sl.items, nil
}

// UnmarshalJSON accepts either an array or a delimited string, mirroring the
// YAML behaviour so the web UI can post whichever is convenient.
func (sl *StringList) UnmarshalJSON(data []byte) error {
	var items []string
	if err := json.Unmarshal(data, &items); err == nil {
		sl.items = items
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		sl.items = splitDelimited(str)
		return nil
	}
	return fmt.Errorf("failed to unmarshal as string or []string")
}

// MarshalJSON always emits an array.
func (sl StringList) MarshalJSON() ([]byte, error) {
	if len(sl.items) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(sl.items)
}

// String joins the items with a pipe.
func (sl *StringList) String() string { return strings.Join(sl.items, "|") }

// Set implements flag.Value. Pipe separated on the command line.
func (sl *StringList) Set(value string) error {
	sl.items = strings.Split(value, "|")
	return nil
}

// splitDelimited applies the historical scalar splitting rules: pipe wins over
// comma, and an empty string yields no items.
func splitDelimited(str string) []string {
	switch {
	case strings.Contains(str, "|"):
		return strings.Split(str, "|")
	case strings.Contains(str, ","):
		return strings.Split(str, ",")
	case str != "":
		return []string{str}
	default:
		return []string{}
	}
}

// Config is the full set of options for a scan. The YAML tags define the
// on-disk config file format; fields tagged "-" are runtime only and are never
// persisted.
type Config struct {
	TiingoToken             string     `yaml:"-" json:"tiingo_token"`
	Logfile                 string     `yaml:"logfile" json:"logfile"`
	Outfile                 string     `yaml:"outfile" json:"outfile"`
	StartDate               string     `yaml:"start_date" json:"start_date"`
	EndDate                 string     `yaml:"end_date" json:"end_date"`
	Filter                  string     `yaml:"filter" json:"filter"`
	Source                  string     `yaml:"source" json:"source"`
	Tickers                 StringList `yaml:"tickers" json:"tickers"`
	Market                  string     `yaml:"market" json:"market"`
	Columns                 StringList `yaml:"columns" json:"columns"`
	DropColumns             string     `yaml:"drop_columns" json:"drop_columns"`
	TargetColumn            string     `yaml:"target_column" json:"target_column"`
	Truncate                int        `yaml:"truncate" json:"truncate"`
	Pivot                   bool       `yaml:"pivot" json:"pivot"`
	SplitPct                float64    `yaml:"split_pct" json:"split_pct"`
	OutputFormats           StringList `yaml:"output_formats" json:"output_formats"`
	ParquetCompression      string     `yaml:"parquet_compression" json:"parquet_compression"`
	ParquetPartitionBy      StringList `yaml:"parquet_partition_by" json:"parquet_partition_by"`
	ParquetPartitionDateFmt string     `yaml:"parquet_partition_date_format" json:"parquet_partition_date_format"`
	ParquetSortBy           StringList `yaml:"parquet_sort_by" json:"parquet_sort_by"`
	ParquetRowGroupSize     int        `yaml:"parquet_row_group_size" json:"parquet_row_group_size"`
}

// Sources lists the supported data sources. Yahoo was dropped when its
// endpoint stopped returning quote data.
var Sources = []string{"tiingo", "tiingo-crypto", "coinbase"}

// Compressions lists the supported parquet compression codecs.
var Compressions = []string{"snappy", "gzip", "zstd", "none"}

// PartitionDateFormats lists the supported date partition granularities.
var PartitionDateFormats = []string{"year", "year,month", "year,month,day"}

// DefaultConfig returns a Config populated with the same defaults the CLI flags
// use.
func DefaultConfig() Config {
	return Config{
		TiingoToken:         os.Getenv("TIINGO_API_TOKEN"),
		Outfile:             "output.csv",
		StartDate:           "2024-01-01",
		EndDate:             time.Now().Format(DateLayout),
		Source:              "tiingo",
		ParquetCompression:  "snappy",
		ParquetRowGroupSize: 100000,
	}
}

// Markets returns the markets go-quote knows how to resolve.
func Markets() []string {
	return quote.ValidMarkets[:]
}

// LoadConfig reads a YAML config file into cfg.
func LoadConfig(filename string, cfg *Config) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filename, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal data: %v", err)
	}
	return nil
}

// SaveConfig writes cfg to a YAML file.
func SaveConfig(path string, cfg *Config) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", path, err)
	}
	defer file.Close()
	encoder := yaml.NewEncoder(file)
	return encoder.Encode(cfg)
}

// MarshalConfig renders cfg as YAML using the exact encoder SaveConfig uses, so
// a rendered preview is byte-identical to what would be written to disk.
func MarshalConfig(cfg *Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// HandleConfig loads path into cfg if it exists, otherwise saves cfg to it.
func HandleConfig(path string, cfg *Config) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return SaveConfig(path, cfg)
	}
	return LoadConfig(path, cfg)
}
