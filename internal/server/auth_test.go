package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/store"
)

func TestLoginSuccessAndWhoAmI(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.users.Create("organizer1", "correct-horse", auth.RoleOrganizer); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "organizer1", "password": "correct-horse"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatalf("expected non-empty token")
	}
	if loginResp.Role != "organizer" {
		t.Fatalf("expected role organizer, got %s", loginResp.Role)
	}

	whoReq := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	whoReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	whoRec := httptest.NewRecorder()
	s.ServeHTTP(whoRec, whoReq)
	if whoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", whoRec.Code)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.users.Create("organizer1", "correct-horse", auth.RoleOrganizer); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "organizer1", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWhoAmIWithoutToken(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestRoleDemotionRevokesExistingSessionPrivileges proves that requireRole
// looks up the user's CURRENT role rather than trusting the role snapshotted
// into the session at login time: an organizer demoted to spectator after
// logging in must no longer be able to exercise organizer-only endpoints
// with their existing (still valid) token.
func TestRoleDemotionRevokesExistingSessionPrivileges(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.DB.Close() })
	engine, err := plugin.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)
	s := New(st, engine)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	// Sanity check: the token currently has organizer privileges.
	body, _ := json.Marshal(map[string]string{
		"name": "Before Demotion", "sport_plugin_id": "dragonboat", "tournament_type_id": "timed-heats-reseeding",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 before demotion, got %d: %s", rec.Code, rec.Body.String())
	}

	// Demote the user directly in the DB, bypassing any application API
	// (there is none) to simulate an admin changing the role out from under
	// an active session.
	if _, err := st.DB.Exec(`UPDATE users SET role = ? WHERE username = ?`, string(auth.RoleSpectator), "organizer1"); err != nil {
		t.Fatalf("demote user: %v", err)
	}

	body2, _ := json.Marshal(map[string]string{
		"name": "After Demotion", "sport_plugin_id": "dragonboat", "tournament_type_id": "timed-heats-reseeding",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 after demotion (old session role must not be trusted), got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+token)
	logoutRec := httptest.NewRecorder()
	s.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", logoutRec.Code, logoutRec.Body.String())
	}

	whoReq := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	whoReq.Header.Set("Authorization", "Bearer "+token)
	whoRec := httptest.NewRecorder()
	s.ServeHTTP(whoRec, whoReq)
	if whoRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", whoRec.Code)
	}
}

func TestWhoAmIWithInvalidToken(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	req.Header.Set("Authorization", "Bearer this-is-not-a-real-token")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
