package round

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

func seedTournament(t *testing.T, s *store.Store) int64 {
	t.Helper()
	res, err := s.DB.Exec(`INSERT INTO tournaments (name, sport_plugin_id, tournament_type_id, language, status) VALUES ('Test', 'dragonboat', 'timed-heats-reseeding', 'en', 'draft')`)
	if err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed tournament id: %v", err)
	}
	return id
}

func TestCreateRoundWithGroups(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	pr, groups, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1", "t2"}, {"t3", "t4"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}
	if pr.ID == 0 {
		t.Fatalf("expected non-zero round ID")
	}
	if pr.Status != StatusOpen {
		t.Fatalf("expected status open, got %s", pr.Status)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	fetched, err := repo.GetRound(pr.ID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if fetched.RoundNumber != 1 {
		t.Fatalf("expected round number 1, got %d", fetched.RoundNumber)
	}

	listed, err := repo.ListGroups(pr.ID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(listed) != 2 || listed[0].TeamIDs[0] != "t1" {
		t.Fatalf("unexpected groups: %+v", listed)
	}
}

func TestSetStatus(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	pr, _, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}

	if err := repo.SetStatus(pr.ID, StatusClosed); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	fetched, err := repo.GetRound(pr.ID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if fetched.Status != StatusClosed {
		t.Fatalf("expected status closed, got %s", fetched.Status)
	}
}

func TestGetRoundNotFound(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.GetRound(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetGroup(t *testing.T) {
	s := newTestStore(t)
	tournamentID := seedTournament(t, s)
	repo := NewRepo(s)

	_, groups, err := repo.CreateRound(tournamentID, 1, [][]string{{"t1", "t2"}})
	if err != nil {
		t.Fatalf("CreateRound: %v", err)
	}

	g, err := repo.GetGroup(groups[0].ID)
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if g.RoundID != groups[0].RoundID || len(g.TeamIDs) != 2 {
		t.Fatalf("unexpected group: %+v", g)
	}

	if _, err := repo.GetGroup(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
