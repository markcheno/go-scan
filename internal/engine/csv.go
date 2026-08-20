package engine

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

// writeToCSV writes the rows, header included, to a CSV file, creating parent
// directories as needed.
func writeToCSV(filename string, allRows [][]string) error {
	if dir := filepath.Dir(filename); dir != "" {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.WriteAll(allRows); err != nil {
		return err
	}
	return writer.Error()
}
