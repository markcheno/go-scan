package engine

import (
	"slices"
	"sort"
	"strings"
)

// BaseHeaders are the columns every run produces before user columns.
var BaseHeaders = []string{"symbol", "date", "open", "high", "low", "close", "volume"}

// dropColumns removes every column whose header case-insensitively matches one
// of names. Indices are collected up front and removed from the right, because
// each removal shifts everything after it.
func dropColumns(data [][]string, names []string) [][]string {
	if len(data) == 0 || len(names) == 0 {
		return data
	}

	var targets []int
	for i, header := range data[0] {
		if containsFold(names, header) {
			targets = append(targets, i)
		}
	}
	return dropColumnsAt(data, targets)
}

// dropColumnsAt removes the given column indices from every row.
func dropColumnsAt(data [][]string, targets []int) [][]string {
	if len(targets) == 0 {
		return data
	}
	sort.Sort(sort.Reverse(sort.IntSlice(targets)))

	out := make([][]string, len(data))
	for i, row := range data {
		newRow := make([]string, 0, len(row))
		newRow = append(newRow, row...)
		for _, col := range targets {
			if col < len(newRow) {
				newRow = slices.Delete(newRow, col, col+1)
			}
		}
		out[i] = newRow
	}
	return out
}

// pivot transforms a long table into a wide one. indexCol becomes the row
// label, the values of pivotCol become column prefixes, and every remaining
// column is emitted once per pivot value as "<pivotValue>_<column>".
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
	indexColPos := slices.Index(headers, indexCol)
	pivotColPos := slices.Index(headers, pivotCol)
	if indexColPos == -1 || pivotColPos == -1 {
		return input // Return original if columns not found
	}

	// Store value column names (everything except index and pivot columns)
	valueColumns := make([]string, 0, len(headers))
	for i, h := range headers {
		if i != indexColPos && i != pivotColPos {
			valueColumns = append(valueColumns, h)
		}
	}

	// Collect unique indices and pivot values
	uniqueIndices := make(map[string]bool)
	uniquePivotVals := make(map[string]bool)
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

	// Create map to store values, keyed index_pivotValue_valueColumn
	valueMap := make(map[string]string)
	for _, row := range input[1:] {
		idx := row[indexColPos]
		pval := row[pivotColPos]
		valIdx := 0
		for i, val := range row {
			if i != indexColPos && i != pivotColPos {
				valueMap[idx+"_"+pval+"_"+valueColumns[valIdx]] = val
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
				row[colIdx] = valueMap[idx+"_"+pval+"_"+valCol]
				colIdx++
			}
		}
		result[i+1] = row
	}

	return result
}

// dropOtherTargets removes every pivoted column containing "target" except the
// one named by keep.
func dropOtherTargets(data [][]string, keep string) [][]string {
	if len(data) == 0 || keep == "" {
		return data
	}
	var targets []int
	for i, header := range data[0] {
		if strings.Contains(header, "target") && header != keep {
			targets = append(targets, i)
		}
	}
	return dropColumnsAt(data, targets)
}

// parseColumnSpecs turns "name=expr" entries into an ordered name/expression
// map. The expression may itself contain "=", as in a >= comparison.
func parseColumnSpecs(specs []string) (*OrderedMap, error) {
	om := NewOrderedMap()
	for _, spec := range specs {
		if strings.TrimSpace(spec) == "" {
			continue
		}
		name, expr, ok := strings.Cut(spec, "=")
		name, expr = strings.TrimSpace(name), strings.TrimSpace(expr)
		if !ok || name == "" || expr == "" {
			return nil, FieldError{Field: "columns", Index: -1, Severity: SeverityError,
				Message: "expected name=expression, got " + spec}
		}
		om.Set(name, expr)
	}
	return om, nil
}
