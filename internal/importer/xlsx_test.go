package importer

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func writeFixtureXLSX(t *testing.T) *bytes.Buffer {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows := [][]string{
		{"name", "club", "boat_class"},
		{"Sturmvögel Dresden", "Sturmvögel Dresden e.V.", "standard"},
		{"Kraken Paddling Club", "", "standard"},
	}
	for i, row := range rows {
		for j, val := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName: %v", err)
			}
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return &buf
}

func TestParseXLSX(t *testing.T) {
	buf := writeFixtureXLSX(t)
	rows, err := ParseXLSX(buf)
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Sturmvögel Dresden" {
		t.Fatalf("unexpected name: %s", rows[0]["name"])
	}
	if rows[1]["club"] != "" {
		t.Fatalf("expected empty club, got %s", rows[1]["club"])
	}
}
