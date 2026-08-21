package store

import (
	"path/filepath"
	"sync"
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
	if count != 5 {
		t.Fatalf("expected 5 migrations applied, got %d", count)
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
	if count != 5 {
		t.Fatalf("expected migrations not reapplied, still 5, got %d", count)
	}
}

// TestForeignKeysEnforced proves that store.Open enables SQLite's
// foreign_keys pragma, so inserting a team referencing a nonexistent
// tournament fails instead of silently creating an orphan row.
func TestForeignKeysEnforced(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.DB.Close()

	_, err = s.DB.Exec(`INSERT INTO teams (tournament_id, name, club, extra_fields) VALUES (?, ?, ?, ?)`,
		999999, "Orphan Team", "", "{}")
	if err == nil {
		t.Fatalf("expected foreign key violation inserting team under nonexistent tournament, got nil error")
	}
}

// TestConcurrentWritesDoNotFailWithBusy proves WAL mode plus a busy_timeout
// let concurrent writers succeed instead of failing with SQLITE_BUSY, which
// is the regression the final review found (no WAL, no busy timeout).
func TestConcurrentWritesDoNotFailWithBusy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.DB.Close()

	if _, err := s.DB.Exec(`INSERT INTO tournaments (id, name, sport_plugin_id, tournament_type_id, language, status) VALUES (1, 'Test', 'dragonboat', 'timed-heats-reseeding', 'en', 'draft')`); err != nil {
		t.Fatalf("seed tournament: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.DB.Exec(`INSERT INTO teams (tournament_id, name, club, extra_fields) VALUES (?, ?, ?, ?)`,
				1, "Team", "", "{}")
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("worker %d: unexpected error: %v", i, err)
		}
	}

	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM teams WHERE tournament_id = 1`).Scan(&count); err != nil {
		t.Fatalf("count teams: %v", err)
	}
	if count != workers {
		t.Fatalf("expected %d teams, got %d", workers, count)
	}
}
