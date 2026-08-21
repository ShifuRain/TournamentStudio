package tournament

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

func TestCreateAndGetTournament(t *testing.T) {
	repo := NewRepo(newTestStore(t))

	created, err := repo.Create(Tournament{
		Name:             "Herbstregatta Rheinauen",
		SportPluginID:    "dragonboat",
		TournamentTypeID: "timed-heats-reseeding",
		Language:         "de",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}
	if created.Status != "draft" {
		t.Fatalf("expected status draft, got %s", created.Status)
	}

	fetched, err := repo.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Name != "Herbstregatta Rheinauen" {
		t.Fatalf("unexpected name: %s", fetched.Name)
	}
}

func TestListTournaments(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.Create(Tournament{Name: "A", SportPluginID: "dragonboat", TournamentTypeID: "timed-heats-reseeding", Language: "en"}); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := repo.Create(Tournament{Name: "B", SportPluginID: "dragonboat", TournamentTypeID: "timed-heats-reseeding", Language: "en"}); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 tournaments, got %d", len(list))
	}
}

func TestGetTournamentNotFound(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.Get(999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
