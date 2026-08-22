package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestNextRoundComputesReseededGroups(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2", "A3", "A4"},
		{"B1", "B2", "B3", "B4"},
	})

	resultsBody, _ := json.Marshal(map[string]any{
		"A1": map[string]any{"time_seconds": 120.0},
		"A2": map[string]any{"time_seconds": 121.0},
		"A3": map[string]any{"time_seconds": 122.0},
		"A4": map[string]any{"time_seconds": 123.0},
		"B1": map[string]any{"time_seconds": 120.0},
		"B2": map[string]any{"time_seconds": 121.0},
		"B3": map[string]any{"time_seconds": 122.0},
		"B4": map[string]any{"time_seconds": 123.0},
	})
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, roundID), nil)
	nextReq.Header.Set("Authorization", "Bearer "+token)
	nextRec := httptest.NewRecorder()
	s.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", nextRec.Code, nextRec.Body.String())
	}

	var next struct {
		RoundNumber int `json:"round_number"`
		Groups      []struct {
			TeamIDs []string `json:"team_ids"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(nextRec.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode next round response: %v", err)
	}
	if next.RoundNumber != 2 {
		t.Fatalf("expected round number 2, got %d", next.RoundNumber)
	}
	if len(next.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(next.Groups))
	}

	wantGroup0 := []string{"A1", "A2", "B3", "B4"}
	wantGroup1 := []string{"A3", "A4", "B1", "B2"}
	if !reflect.DeepEqual(next.Groups[0].TeamIDs, wantGroup0) {
		t.Fatalf("expected group 0 team_ids %v, got %v", wantGroup0, next.Groups[0].TeamIDs)
	}
	if !reflect.DeepEqual(next.Groups[1].TeamIDs, wantGroup1) {
		t.Fatalf("expected group 1 team_ids %v, got %v", wantGroup1, next.Groups[1].TeamIDs)
	}
}

func TestNextRoundRejectsOpenRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	nextReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, roundID), nil)
	nextReq.Header.Set("Authorization", "Bearer "+token)
	nextRec := httptest.NewRecorder()
	s.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", nextRec.Code)
	}
}
