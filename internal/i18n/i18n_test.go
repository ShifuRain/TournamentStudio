package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslateBuiltIn(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("de", "next_heat"); got != "NÄCHSTER" {
		t.Fatalf("expected NÄCHSTER, got %s", got)
	}
	if got := c.Translate("en", "next_heat"); got != "NEXT" {
		t.Fatalf("expected NEXT, got %s", got)
	}
}

func TestTranslateFallsBackToEnglish(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("fr", "next_heat"); got != "NEXT" {
		t.Fatalf("expected fallback to NEXT, got %s", got)
	}
}

func TestTranslateUnknownKeyReturnsKey(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("en", "nonexistent_key"); got != "nonexistent_key" {
		t.Fatalf("expected key echoed back, got %s", got)
	}
}

func TestExternalLanguageDropIn(t *testing.T) {
	dir := t.TempDir()
	frFile := filepath.Join(dir, "fr.json")
	if err := os.WriteFile(frFile, []byte(`{"next_heat": "SUIVANT"}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Translate("fr", "next_heat"); got != "SUIVANT" {
		t.Fatalf("expected SUIVANT, got %s", got)
	}
}
