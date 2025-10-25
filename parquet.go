package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apache/arrow/go/v18/arrow"
	"github.com/apache/arrow/go/v18/arrow/array"
	"github.com/apache/arrow/go/v18/arrow/memory"
	"github.com/apache/arrow/go/v18/parquet"
	"github.com/apache/arrow/go/v18/parquet/compress"
	"github.com/apache/arrow/go/v18/parquet/pqarrow"
)

// ColumnType represents the detected type of a column
type ColumnType int

const (
	TypeString ColumnType = iota
	TypeFloat64
	TypeTimestamp
)

// detectColumnType analyzes sample rows to infer the column type
func detectColumnType(colName string, values []string) ColumnType {
	// Date column is always timestamp
	if colName == "date" {
		return TypeTimestamp
	}

	// Symbol column is always string
	if colName == "symbol" {
		return TypeString
	}

	// Try to parse as float64 for a few samples
	floatCount := 0
	sampleSize := min(len(values), 100)
	for i := 0; i < sampleSize; i++ {
		if _, err := strconv.ParseFloat(values[i], 64); err == nil {
			floatCount++
		}
	}

	// If most samples are float, consider it float
	if float64(floatCount)/float64(sampleSize) > 0.9 {
		return TypeFloat64
	}

	return TypeString
}

// inferArrowSchema creates an Arrow schema from headers and sample data
func inferArrowSchema(headers []string, sampleRows [][]string) *arrow.Schema {
	fields := make([]arrow.Field, len(headers))

	for i, header := range headers {
		// Collect sample values for this column
		values := make([]string, 0, len(sampleRows))
		for _, row := range sampleRows {
			if i < len(row) {
				values = append(values, row[i])
			}
		}

		colType := detectColumnType(header, values)

		switch colType {
		case TypeTimestamp:
			fields[i] = arrow.Field{Name: header, Type: arrow.FixedWidthTypes.Timestamp_ms, Nullable: true}
		case TypeFloat64:
			fields[i] = arrow.Field{Name: header, Type: arrow.PrimitiveTypes.Float64, Nullable: true}
		case TypeString:
			fields[i] = arrow.Field{Name: header, Type: arrow.BinaryTypes.String, Nullable: true}
		}
	}

	return arrow.NewSchema(fields, nil)
}

// convertRowsToArrowTable converts [][]string to an Arrow Table with proper types
func convertRowsToArrowTable(headers []string, rows [][]string, schema *arrow.Schema) (arrow.Table, error) {
	pool := memory.NewGoAllocator()
	builders := make([]array.Builder, len(headers))

	// Create builders for each column based on schema
	for i, field := range schema.Fields() {
		switch field.Type.(type) {
		case *arrow.TimestampType:
			builders[i] = array.NewTimestampBuilder(pool, field.Type.(*arrow.TimestampType))
		case *arrow.Float64Type:
			builders[i] = array.NewFloat64Builder(pool)
		case *arrow.StringType:
			builders[i] = array.NewStringBuilder(pool)
		default:
			builders[i] = array.NewStringBuilder(pool)
		}
	}

	// Populate builders with data
	for _, row := range rows {
		for i, val := range row {
			if i >= len(builders) {
				continue
			}

			switch b := builders[i].(type) {
			case *array.TimestampBuilder:
				// Parse date string to timestamp
				if t, err := time.Parse("2006-01-02", val); err == nil {
					b.Append(arrow.Timestamp(t.UnixMilli()))
				} else {
					b.AppendNull()
				}
			case *array.Float64Builder:
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					b.Append(f)
				} else {
					b.AppendNull()
				}
			case *array.StringBuilder:
				b.Append(val)
			}
		}
	}

	// Build arrays
	columns := make([]arrow.Column, len(headers))
	for i, builder := range builders {
		arr := builder.NewArray()
		defer arr.Release()
		chunked := arrow.NewChunked(schema.Field(i).Type, []arrow.Array{arr})
		columns[i] = *arrow.NewColumn(schema.Field(i), chunked)
	}

	return array.NewTable(schema, columns, int64(len(rows))), nil
}

// sortDataByColumns sorts the data rows based on specified sort columns
func sortDataByColumns(headers []string, rows [][]string, sortColumns []string) [][]string {
	if len(sortColumns) == 0 || len(rows) == 0 {
		return rows
	}

	// Find column indices for sort columns
	sortIndices := make([]int, 0, len(sortColumns))
	for _, sortCol := range sortColumns {
		for i, header := range headers {
			if strings.EqualFold(header, sortCol) {
				sortIndices = append(sortIndices, i)
				break
			}
		}
	}

	if len(sortIndices) == 0 {
		return rows
	}

	// Sort rows
	sort.Slice(rows, func(i, j int) bool {
		for _, idx := range sortIndices {
			if idx >= len(rows[i]) || idx >= len(rows[j]) {
				continue
			}

			// Try numeric comparison first
			vi, erri := strconv.ParseFloat(rows[i][idx], 64)
			vj, errj := strconv.ParseFloat(rows[j][idx], 64)

			if erri == nil && errj == nil {
				if vi != vj {
					return vi < vj
				}
				continue
			}

			// Fall back to string comparison
			if rows[i][idx] != rows[j][idx] {
				return rows[i][idx] < rows[j][idx]
			}
		}
		return false
	})

	return rows
}

// partitionDataByColumns groups rows by partition columns
// Returns a map where keys are Hive-style partition paths (e.g., "symbol=AAPL/year=2024")
func partitionDataByColumns(headers []string, rows [][]string, partitionCols []string, dateFormat string) map[string][][]string {
	if len(partitionCols) == 0 {
		// No partitioning - return all data under empty key
		return map[string][][]string{
			"": rows,
		}
	}

	partitions := make(map[string][][]string)

	// Find partition column indices
	partitionIndices := make(map[string]int)
	for _, partCol := range partitionCols {
		for i, header := range headers {
			if strings.EqualFold(header, partCol) {
				partitionIndices[partCol] = i
				break
			}
		}
	}

	// Group rows by partition values
	for _, row := range rows {
		keyParts := make(map[string]string)

		for partCol, idx := range partitionIndices {
			if idx >= len(row) {
				continue
			}

			value := row[idx]

			// Handle date partitioning with format
			if partCol == "date" && dateFormat != "" {
				if t, err := time.Parse("2006-01-02", value); err == nil {
					dateParts := strings.Split(dateFormat, ",")
					for _, part := range dateParts {
						part = strings.TrimSpace(part)
						switch part {
						case "year":
							keyParts["year"] = fmt.Sprintf("%04d", t.Year())
						case "month":
							keyParts["month"] = fmt.Sprintf("%02d", t.Month())
						case "day":
							keyParts["day"] = fmt.Sprintf("%02d", t.Day())
						}
					}
				}
			} else {
				keyParts[partCol] = value
			}
		}

		// Build Hive-style partition key string
		partKeys := make([]string, 0, len(keyParts))
		for k := range keyParts {
			partKeys = append(partKeys, k)
		}
		sort.Strings(partKeys)

		pathParts := make([]string, 0, len(partKeys))
		for _, k := range partKeys {
			pathParts = append(pathParts, fmt.Sprintf("%s=%s", k, keyParts[k]))
		}
		key := strings.Join(pathParts, "/")

		partitions[key] = append(partitions[key], row)
	}

	return partitions
}

// getCompressionCodec returns the Parquet compression codec
func getCompressionCodec(compressionType string) compress.Compression {
	switch strings.ToLower(compressionType) {
	case "gzip":
		return compress.Codecs.Gzip
	case "snappy":
		return compress.Codecs.Snappy
	case "zstd":
		return compress.Codecs.Zstd
	case "none", "uncompressed":
		return compress.Codecs.Uncompressed
	default:
		return compress.Codecs.Snappy
	}
}

// writeToParquet writes data to Parquet file(s) with optional partitioning
func writeToParquet(filename string, allRows [][]string, config *ScanFlags) error {
	if len(allRows) < 2 {
		return fmt.Errorf("insufficient data to write")
	}

	headers := allRows[0]
	dataRows := allRows[1:]

	// Sort data if requested
	if len(config.ParquetSortBy.items) > 0 {
		log.Printf("Sorting by: %v", config.ParquetSortBy.items)
		dataRows = sortDataByColumns(headers, dataRows, config.ParquetSortBy.items)
	}

	// Partition data
	partitions := partitionDataByColumns(headers, dataRows, config.ParquetPartitionBy.items, config.ParquetPartitionDateFmt)

	// Infer schema from sample data
	sampleSize := min(len(dataRows), 1000)
	schema := inferArrowSchema(headers, dataRows[:sampleSize])

	// Get base directory from filename
	baseDir := strings.TrimSuffix(filename, filepath.Ext(filename))
	if len(partitions) == 1 {
		// No partitioning or single partition - write single file
		for key, rows := range partitions {
			outputPath := filename
			if key != "" {
				outputPath = filepath.Join(baseDir, key, "data.parquet")
			}
			if err := writeSingleParquetFile(outputPath, headers, rows, schema, config); err != nil {
				return fmt.Errorf("failed to write parquet file: %w", err)
			}
		}
	} else {
		// Multiple partitions - write Hive-style partitioned data
		for key, rows := range partitions {
			partitionPath := filepath.Join(baseDir, key, "data.parquet")
			if err := writeSingleParquetFile(partitionPath, headers, rows, schema, config); err != nil {
				return fmt.Errorf("failed to write partition %s: %w", key, err)
			}
		}
	}

	return nil
}

// writeSingleParquetFile writes a single Parquet file
func writeSingleParquetFile(filename string, headers []string, rows [][]string, schema *arrow.Schema, config *ScanFlags) error {
	// Create directory if needed
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	// Convert to Arrow table
	table, err := convertRowsToArrowTable(headers, rows, schema)
	if err != nil {
		return fmt.Errorf("failed to convert to Arrow table: %w", err)
	}
	defer table.Release()

	// Configure Parquet writer properties
	writerProps := parquet.NewWriterProperties(
		parquet.WithCompression(getCompressionCodec(config.ParquetCompression)),
	)

	// Configure Arrow writer properties
	arrowProps := pqarrow.DefaultWriterProps()

	// Create Parquet writer
	writer, err := pqarrow.NewFileWriter(schema, file, writerProps, arrowProps)
	if err != nil {
		return fmt.Errorf("failed to create Parquet writer: %w", err)
	}
	defer writer.Close()

	// Write table
	if err := writer.WriteTable(table, int64(config.ParquetRowGroupSize)); err != nil {
		return fmt.Errorf("failed to write table: %w", err)
	}

	log.Printf("Writing to %s (%d rows)", filename, len(rows))
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
