package main

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestFlagDefaults(t *testing.T) {
	// Reset flags before test
	flags = ScanFlags{}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	initFlags()

	tests := []struct {
		name     string
		flag     interface{}
		expected interface{}
	}{
		{"StartDate", flags.StartDate, "2024-01-01"},
		{"EndDate", flags.EndDate, time.Now().Format("2006-01-02")},
		{"Source", flags.Source, "tiingo"},
		{"Outfile", flags.Outfile, "output.csv"},
		{"Market", flags.Market, ""},
		{"Truncate", flags.Truncate, 0},
		{"Pivot", flags.Pivot, false},
		{"SplitPct", flags.SplitPct, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.flag != tt.expected {
				t.Errorf("got %v, want %v", tt.flag, tt.expected)
			}
		})
	}
}

func TestStringListFlag(t *testing.T) {
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
			sl := &StringList{}
			err := sl.Set(tt.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(sl.items) != len(tt.expected) {
				t.Errorf("got %d items, want %d", len(sl.items), len(tt.expected))
			}
			for i, item := range sl.items {
				if item != tt.expected[i] {
					t.Errorf("item[%d] = %s, want %s", i, item, tt.expected[i])
				}
			}
		})
	}
}

func TestConfigFileHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-config")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "test_config.yaml")

	// Create test configuration with explicit values
	testFlags := &ScanFlags{
		StartDate: "2024-01-01",
		EndDate:   "2024-12-31",
		Tickers:   StringList{items: []string{"AAPL", "GOOGL"}},
		Source:    "yahoo",
	}

	// Test saving config
	if err := saveConfig(tempFile, testFlags); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load config into new struct
	loadedFlags := &ScanFlags{}
	if err := loadConfig(tempFile, loadedFlags); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded values
	if loadedFlags.StartDate != testFlags.StartDate {
		t.Errorf("StartDate = %s, want %s", loadedFlags.StartDate, testFlags.StartDate)
	}
	if loadedFlags.Source != testFlags.Source {
		t.Errorf("Source = %s, want %s", loadedFlags.Source, testFlags.Source)
	}
	if !reflect.DeepEqual(loadedFlags.Tickers.items, testFlags.Tickers.items) {
		t.Errorf("Tickers = %v, want %v", loadedFlags.Tickers.items, testFlags.Tickers.items)
	}
}
func TestHandleConfig(t *testing.T) {
	tempFile := "test_handle_config.yaml"
	defer os.Remove(tempFile)

	tests := []struct {
		name        string
		configFile  string
		shouldError bool
	}{
		{"EmptyConfig", "", false},
		{"ValidConfig", tempFile, false},
		{"InvalidConfig", "/nonexistent/path/config.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags.ConfigFile = tt.configFile
			err := handleConfig(&flags)
			if (err != nil) != tt.shouldError {
				t.Errorf("handleConfig() error = %v, shouldError %v", err, tt.shouldError)
			}
		})
	}
}
