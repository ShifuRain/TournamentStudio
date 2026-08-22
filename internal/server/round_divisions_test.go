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

func TestComputeDivisionsSplitsRankedTeams(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2", "t3", "t4"}})

	resultsBody, _ := json.Marshal(map[string]any{
		"t1": map[string]any{"time_seconds": 120.0},
		"t2": map[string]any{"time_seconds": 121.0},
		"t3": map[string]any{"time_seconds": 122.0},
		"t4": map[string]any{"time_seconds": 123.0},
	})
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{
			{"name": "Gold Final", "size": 2},
		},
	})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}

	var resp struct {
		Divisions []struct {
			Name    string   `json:"name"`
			TeamIDs []string `json:"team_ids"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Divisions) != 2 {
		t.Fatalf("expected 2 divisions (Gold Final + implicit Final), got %d", len(resp.Divisions))
	}
	if resp.Divisions[0].Name != "Gold Final" || len(resp.Divisions[0].TeamIDs) != 2 {
		t.Fatalf("unexpected first division: %+v", resp.Divisions[0])
	}
	if resp.Divisions[0].TeamIDs[0] != "t1" || resp.Divisions[0].TeamIDs[1] != "t2" {
		t.Fatalf("expected Gold Final to be the fastest two teams, got %v", resp.Divisions[0].TeamIDs)
	}
}
