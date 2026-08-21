package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func loginAs(t *testing.T, s *Server, username, password string, role auth.Role) string {
	t.Helper()
	if _, err := s.users.Create(username, password, role); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return resp.Token
}

func TestCreateAndListTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	body, _ := json.Marshal(map[string]string{
		"name":               "Herbstregatta Rheinauen",
		"sport_plugin_id":    "dragonboat",
		"tournament_type_id": "timed-heats-reseeding",
		"language":           "de",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/tournaments", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var list []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 tournament, got %d", len(list))
	}
}

func TestCreateTournamentForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "spec1", "pw", auth.RoleSpectator)

	body, _ := json.Marshal(map[string]string{"name": "X", "sport_plugin_id": "dragonboat", "tournament_type_id": "timed-heats-reseeding"})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
