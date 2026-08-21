package importer

import (
	"encoding/csv"
	"fmt"
	"io"
)

func ParseCSV(r io.Reader) ([]map[string]string, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	header := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				row[col] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
