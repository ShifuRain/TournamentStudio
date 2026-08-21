package importer

import (
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := "name,club,boat_class\nMöwe RC Kiel,Möwe Ruderclub e.V.,standard\nWassermann Berlin,,standard\n"
	rows, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Möwe RC Kiel" {
		t.Fatalf("unexpected name: %s", rows[0]["name"])
	}
}

func TestValidateFlagsMissingName(t *testing.T) {
	rows := []map[string]string{
		{"name": "Rhein Dragons Köln", "club": "Rhein Dragons Köln e.V."},
		{"name": "", "club": "No Name Rowing"},
	}
	result := Validate(1, rows)
	if len(result.Teams) != 1 {
		t.Fatalf("expected 1 valid team, got %d", len(result.Teams))
	}
	if len(result.Problems) != 1 {
		t.Fatalf("expected 1 problem, got %d", len(result.Problems))
	}
	if result.Problems[0].RowIndex != 1 {
		t.Fatalf("expected problem at row index 1, got %d", result.Problems[0].RowIndex)
	}
}

func TestValidateCapturesExtraFields(t *testing.T) {
	rows := []map[string]string{
		{"name": "Nixe Hamburg", "club": "Nixe Hamburg e.V.", "boat_class": "standard"},
	}
	result := Validate(1, rows)
	if result.Teams[0].ExtraFields["boat_class"] != "standard" {
		t.Fatalf("expected boat_class standard, got %v", result.Teams[0].ExtraFields)
	}
}
