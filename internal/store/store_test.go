package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrationsOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.DB.Close()

	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 migrations applied, got %d", count)
	}

	// Reopening the same database must not reapply migrations.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.DB.Close()

	if err := s2.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected migrations not reapplied, still 2, got %d", count)
	}
}
