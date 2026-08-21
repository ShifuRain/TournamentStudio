package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func createTestTournament(t *testing.T, s *Server, token string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"name":               "Herbstregatta Rheinauen",
		"sport_plugin_id":    "dragonboat",
		"tournament_type_id": "timed-heats-reseeding",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	var created struct{ ID int64 }
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created tournament: %v", err)
	}
	return created.ID
}

func TestCreateAndListTeamsManually(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	body, _ := json.Marshal(map[string]any{
		"name":         "Rhein Dragons Köln",
		"club":         "Rhein Dragons Köln e.V.",
		"extra_fields": map[string]string{"boat_class": "standard"},
	})
	path := fmt.Sprintf("/api/tournaments/%d/teams", tournamentID)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, path, nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)

	var teams []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &teams); err != nil {
		t.Fatalf("decode teams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
}
