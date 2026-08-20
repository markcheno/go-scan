package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParquetWriting(t *testing.T) {
	// Create test data
	headers := []string{"symbol", "date", "open", "high", "low", "close", "volume"}
	rows := [][]string{
		headers,
		{"AAPL", "2024-01-01", "150.0", "152.0", "149.0", "151.0", "1000000"},
		{"AAPL", "2024-01-02", "151.0", "153.0", "150.0", "152.0", "1100000"},
		{"GOOG", "2024-01-01", "100.0", "102.0", "99.0", "101.0", "500000"},
		{"GOOG", "2024-01-02", "101.0", "103.0", "100.0", "102.0", "550000"},
	}

	// Create test config
	config := &Config{
		ParquetCompression:  "snappy",
		ParquetRowGroupSize: 100,
	}

	// Test 1: Write single Parquet file without partitioning
	t.Run("Single file without partitioning", func(t *testing.T) {
		testFile := filepath.Join(t.TempDir(), "test.parquet")
		_, err := writeToParquet(testFile, rows, config)
		if err != nil {
			t.Fatalf("Failed to write Parquet: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Fatalf("Parquet file was not created")
		}

		// Check file size is reasonable
		info, _ := os.Stat(testFile)
		if info.Size() < 100 {
			t.Fatalf("Parquet file is too small: %d bytes", info.Size())
		}
	})

	// Test 2: Write with partitioning by symbol
	t.Run("Partitioned by symbol", func(t *testing.T) {
		testFile := filepath.Join(t.TempDir(), "test.parquet")
		config.ParquetPartitionBy = NewStringList("symbol")

		_, err := writeToParquet(testFile, rows, config)
		if err != nil {
			t.Fatalf("Failed to write partitioned Parquet: %v", err)
		}

		// Verify partition directories exist
		baseDir := filepath.Dir(testFile)
		baseName := filepath.Base(testFile)
		baseName = baseName[:len(baseName)-len(filepath.Ext(baseName))]

		aaplPath := filepath.Join(baseDir, baseName, "symbol=AAPL", "data.parquet")
		googPath := filepath.Join(baseDir, baseName, "symbol=GOOG", "data.parquet")

		if _, err := os.Stat(aaplPath); os.IsNotExist(err) {
			t.Fatalf("AAPL partition file was not created at %s", aaplPath)
		}
		if _, err := os.Stat(googPath); os.IsNotExist(err) {
			t.Fatalf("GOOG partition file was not created at %s", googPath)
		}
	})

	// Test 3: Write with sorting
	t.Run("Sorted by date", func(t *testing.T) {
		testFile := filepath.Join(t.TempDir(), "test_sorted.parquet")
		config.ParquetPartitionBy = NewStringList()
		config.ParquetSortBy = NewStringList("date")

		_, err := writeToParquet(testFile, rows, config)
		if err != nil {
			t.Fatalf("Failed to write sorted Parquet: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Fatalf("Sorted Parquet file was not created")
		}
	})
}

func TestSchemaInference(t *testing.T) {
	headers := []string{"symbol", "date", "close", "volume"}
	sampleRows := [][]string{
		{"AAPL", "2024-01-01", "150.5", "1000000"},
		{"GOOG", "2024-01-02", "100.25", "500000"},
	}

	schema := inferArrowSchema(headers, sampleRows)

	if len(schema.Fields()) != 4 {
		t.Fatalf("Expected 4 fields, got %d", len(schema.Fields()))
	}

	// Check field types
	symbolField := schema.Field(0)
	if symbolField.Name != "symbol" {
		t.Errorf("Expected first field to be 'symbol', got '%s'", symbolField.Name)
	}

	dateField := schema.Field(1)
	if dateField.Name != "date" {
		t.Errorf("Expected second field to be 'date', got '%s'", dateField.Name)
	}
}

func TestSortData(t *testing.T) {
	headers := []string{"symbol", "date", "close"}
	rows := [][]string{
		{"GOOG", "2024-01-02", "102.0"},
		{"AAPL", "2024-01-01", "151.0"},
		{"AAPL", "2024-01-02", "152.0"},
		{"GOOG", "2024-01-01", "101.0"},
	}

	// Sort by date, then symbol
	sorted := sortDataByColumns(headers, rows, []string{"date", "symbol"})

	// Verify order
	expected := [][]string{
		{"AAPL", "2024-01-01", "151.0"},
		{"GOOG", "2024-01-01", "101.0"},
		{"AAPL", "2024-01-02", "152.0"},
		{"GOOG", "2024-01-02", "102.0"},
	}

	for i, row := range sorted {
		for j, val := range row {
			if val != expected[i][j] {
				t.Errorf("Row %d, Col %d: expected %s, got %s", i, j, expected[i][j], val)
			}
		}
	}
}

func TestPartitionDataByColumns(t *testing.T) {
	headers := []string{"symbol", "date", "close"}
	rows := [][]string{
		{"AAPL", "2024-01-01", "151.0"},
		{"AAPL", "2024-01-02", "152.0"},
		{"GOOG", "2024-01-01", "101.0"},
		{"GOOG", "2024-01-02", "102.0"},
	}

	// Partition by symbol
	partitions := partitionDataByColumns(headers, rows, []string{"symbol"}, "")

	if len(partitions) != 2 {
		t.Fatalf("Expected 2 partitions, got %d", len(partitions))
	}

	// Check AAPL partition
	aaplKey := "symbol=AAPL"
	if aaplRows, ok := partitions[aaplKey]; ok {
		if len(aaplRows) != 2 {
			t.Errorf("Expected 2 rows in AAPL partition, got %d", len(aaplRows))
		}
	} else {
		t.Errorf("AAPL partition not found")
	}

	// Check GOOG partition
	googKey := "symbol=GOOG"
	if googRows, ok := partitions[googKey]; ok {
		if len(googRows) != 2 {
			t.Errorf("Expected 2 rows in GOOG partition, got %d", len(googRows))
		}
	} else {
		t.Errorf("GOOG partition not found")
	}
}
