package auth

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

func TestCreateAndFindUser(t *testing.T) {
	repo := NewRepo(newTestStore(t))

	u, err := repo.Create("organizer1", "correct-horse", RoleOrganizer)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}

	found, err := repo.FindByUsername("organizer1")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if found.Role != RoleOrganizer {
		t.Fatalf("expected role organizer, got %s", found.Role)
	}
	if !CheckPassword(found.PasswordHash, "correct-horse") {
		t.Fatalf("expected password to verify")
	}
	if CheckPassword(found.PasswordHash, "wrong-password") {
		t.Fatalf("expected wrong password to fail verification")
	}
}

func TestCreateDuplicateUsername(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.Create("dup", "pw", RoleSpectator); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := repo.Create("dup", "pw2", RoleSpectator); err != ErrDuplicateUsername {
		t.Fatalf("expected ErrDuplicateUsername, got %v", err)
	}
}

func TestFindByUsernameNotFound(t *testing.T) {
	repo := NewRepo(newTestStore(t))
	if _, err := repo.FindByUsername("nobody"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
