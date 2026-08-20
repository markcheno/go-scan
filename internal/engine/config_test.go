package engine

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestStringListSet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"Empty", "", []string{""}},
		{"Single", "AAPL", []string{"AAPL"}},
		{"PipeSeparated", "AAPL|GOOGL", []string{"AAPL", "GOOGL"}},
		{"MultipleItems", "AAPL|GOOGL|MSFT", []string{"AAPL", "GOOGL", "MSFT"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sl StringList
			if err := sl.Set(tt.input); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(sl.Items(), tt.expected) {
				t.Errorf("got %v, want %v", sl.Items(), tt.expected)
			}
		})
	}
}

func TestStringListUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected []string
	}{
		{"Sequence", "tickers:\n  - AAPL\n  - GOOG\n", []string{"AAPL", "GOOG"}},
		{"CommaScalar", "tickers: AAPL,GOOG\n", []string{"AAPL", "GOOG"}},
		{"PipeScalar", "tickers: AAPL|GOOG\n", []string{"AAPL", "GOOG"}},
		{"PipeWinsOverComma", "tickers: A,B|C\n", []string{"A,B", "C"}},
		{"SingleScalar", "tickers: AAPL\n", []string{"AAPL"}},
		{"EmptyScalar", "tickers: \"\"\n", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if err := yaml.Unmarshal([]byte(tt.yaml), &cfg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(cfg.Tickers.Items(), tt.expected) {
				t.Errorf("got %v, want %v", cfg.Tickers.Items(), tt.expected)
			}
		})
	}
}

func TestStringListJSON(t *testing.T) {
	var sl StringList
	if err := json.Unmarshal([]byte(`["AAPL","GOOG"]`), &sl); err != nil {
		t.Fatalf("array: %v", err)
	}
	if !reflect.DeepEqual(sl.Items(), []string{"AAPL", "GOOG"}) {
		t.Errorf("array form: got %v", sl.Items())
	}

	if err := json.Unmarshal([]byte(`"AAPL,GOOG"`), &sl); err != nil {
		t.Fatalf("scalar: %v", err)
	}
	if !reflect.DeepEqual(sl.Items(), []string{"AAPL", "GOOG"}) {
		t.Errorf("scalar form: got %v", sl.Items())
	}

	out, err := json.Marshal(NewStringList())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != "[]" {
		t.Errorf("empty list marshalled as %s, want []", out)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	original := &Config{
		StartDate:     "2024-01-01",
		EndDate:       "2024-12-31",
		Tickers:       NewStringList("AAPL", "GOOGL"),
		Columns:       NewStringList("sma20=sma(c,20)"),
		Source:        "tiingo",
		OutputFormats: NewStringList("csv", "parquet"),
		SplitPct:      0.8,
	}

	if err := SaveConfig(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded := &Config{}
	if err := LoadConfig(path, loaded); err != nil {
		t.Fatalf("load: %v", err)
	}

	// Compare the rendered form: an absent list unmarshals to an empty slice
	// rather than a nil one, which is not a meaningful difference.
	before, _ := MarshalConfig(original)
	after, _ := MarshalConfig(loaded)
	if string(before) != string(after) {
		t.Errorf("round trip changed the config:\n got %s\nwant %s", after, before)
	}
}

func TestMarshalConfigMatchesSaveConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tickers = NewStringList("AAPL")

	rendered, err := MarshalConfig(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The token is runtime-only and must never reach the file.
	cfg.TiingoToken = "secret"
	rendered2, err := MarshalConfig(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(rendered) != string(rendered2) {
		t.Error("the Tiingo token leaked into the rendered config")
	}
	if strings.Contains(string(rendered), "secret") {
		t.Error("rendered config contains the token")
	}
}

func TestHandleConfigSavesThenLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.yaml")

	cfg := DefaultConfig()
	cfg.Tickers = NewStringList("SPY")
	if err := HandleConfig(path, &cfg); err != nil {
		t.Fatalf("save leg: %v", err)
	}

	loaded := Config{}
	if err := HandleConfig(path, &loaded); err != nil {
		t.Fatalf("load leg: %v", err)
	}
	if !reflect.DeepEqual(loaded.Tickers.Items(), []string{"SPY"}) {
		t.Errorf("got %v", loaded.Tickers.Items())
	}

	if err := HandleConfig("", &cfg); err != nil {
		t.Errorf("empty path should be a no-op, got %v", err)
	}
	if err := HandleConfig("/nonexistent/dir/config.yaml", &cfg); err == nil {
		t.Error("expected an error writing to a nonexistent directory")
	}
}
