package team

import (
	"path/filepath"
	"testing"

	"tournamentstudio/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

func TestCreateAndListTeams(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.DB.Exec(`INSERT INTO tournaments (id, name, sport_plugin_id, tournament_type_id, language, status) VALUES (1, 'Test', 'dragonboat', 'timed-heats-reseeding', 'en', 'draft')`); err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	repo := NewRepo(s)

	created, err := repo.Create(Team{
		TournamentID: 1,
		Name:         "Möwe RC Kiel",
		Club:         "Möwe Ruderclub e.V.",
		ExtraFields:  map[string]string{"boat_class": "standard"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}

	teams, err := repo.ListByTournament(1)
	if err != nil {
		t.Fatalf("ListByTournament: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if teams[0].ExtraFields["boat_class"] != "standard" {
		t.Fatalf("expected boat_class standard, got %v", teams[0].ExtraFields)
	}
}
