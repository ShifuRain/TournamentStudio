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

func TestUpdateHeatOverridesPlannedStart(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	body, _ := json.Marshal(map[string]any{"planned_start": "2026-09-01T15:30:00Z"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/heats/%d", tournamentID, heatID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PlannedStart string `json:"planned_start"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PlannedStart != "2026-09-01T15:30:00Z" {
		t.Fatalf("expected planned_start 2026-09-01T15:30:00Z, got %s", resp.PlannedStart)
	}
}

func TestUpdateHeatNotFoundForWrongTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentA := createTestTournament(t, s, token)
	tournamentB := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentA, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentA, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	body, _ := json.Marshal(map[string]any{"planned_start": "2026-09-01T15:30:00Z"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/heats/%d", tournamentB, heatID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpdateHeatForbiddenForTimeEntry(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, _ := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, organizerToken, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{"planned_start": "2026-09-01T15:30:00Z"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/heats/%d", tournamentID, heatID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
