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

func TestMalformedExternalFileDoesNotWipeBuiltIn(t *testing.T) {
	dir := t.TempDir()

	// Write a valid French translation
	frFile := filepath.Join(dir, "fr.json")
	if err := os.WriteFile(frFile, []byte(`{"next_heat": "SUIVANT"}`), 0o644); err != nil {
		t.Fatalf("write fr.json: %v", err)
	}

	// Write a malformed JSON file
	badFile := filepath.Join(dir, "xx.json")
	if err := os.WriteFile(badFile, []byte(`{invalid json`), 0o644); err != nil {
		t.Fatalf("write xx.json: %v", err)
	}

	c, err := Load(dir)

	// Load should return an error (malformed file was skipped)
	if err == nil {
		t.Fatalf("expected non-nil error due to malformed xx.json, got nil")
	}

	// Catalog should still be populated with built-in bundles
	if got := c.Translate("en", "next_heat"); got != "NEXT" {
		t.Fatalf("expected built-in English to still work, got %s", got)
	}
	if got := c.Translate("de", "next_heat"); got != "NÄCHSTER" {
		t.Fatalf("expected built-in German to still work, got %s", got)
	}

	// Valid external language should still be loaded
	if got := c.Translate("fr", "next_heat"); got != "SUIVANT" {
		t.Fatalf("expected valid external French to still work, got %s", got)
	}

	// Error message should mention the malformed file
	if !contains(err.Error(), "xx.json") {
		t.Fatalf("expected error to mention xx.json, got: %v", err)
	}
}

func TestPartialLanguageFallback(t *testing.T) {
	dir := t.TempDir()

	// Write a Spanish translation with only "next_heat", missing "on_time"
	esFile := filepath.Join(dir, "es.json")
	if err := os.WriteFile(esFile, []byte(`{"next_heat": "SIGUIENTE"}`), 0o644); err != nil {
		t.Fatalf("write es.json: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Spanish translation should work for key that exists
	if got := c.Translate("es", "next_heat"); got != "SIGUIENTE" {
		t.Fatalf("expected SIGUIENTE, got %s", got)
	}

	// Should fall back to English for missing key
	if got := c.Translate("es", "on_time"); got != "ON TIME" {
		t.Fatalf("expected fallback to English ON TIME, got %s", got)
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
