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

func TestCreateRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	ids := createRealTeams(t, s, tournamentID, []string{"t1", "t2", "t3", "t4"})

	body, _ := json.Marshal(map[string]any{
		"round_number": 1,
		"groups":       [][]string{{ids["t1"], ids["t2"]}, {ids["t3"], ids["t4"]}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Groups []struct {
			TeamIDs []string `json:"team_ids"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "open" {
		t.Fatalf("expected status open, got %s", resp.Status)
	}
	if len(resp.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Groups))
	}
}

func TestCreateRoundRejectsUnknownTeamID(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	ids := createRealTeams(t, s, tournamentID, []string{"t1"})

	// t1 is a real team; "does-not-exist" is not a team of this
	// tournament at all.
	body, _ := json.Marshal(map[string]any{
		"round_number": 1,
		"groups":       [][]string{{ids["t1"], "does-not-exist"}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown team_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRoundNonexistentTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	body, _ := json.Marshal(map[string]any{
		"round_number": 1,
		"groups":       [][]string{{"t1"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tournaments/999999/rounds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent tournament, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRoundForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)

	body, _ := json.Marshal(map[string]any{"round_number": 1, "groups": [][]string{{"t1"}}})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
