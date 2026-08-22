package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/round"
)

// scheduleTestRound schedules every group of roundID onto a single
// freshly created course and returns a map from that group's ID to its
// new heat's ID.
func scheduleTestRound(t *testing.T, s *Server, token string, tournamentID, roundID int64) map[int64]int64 {
	t.Helper()
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)
	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	assignments := make([]map[string]any, len(groups))
	for i, g := range groups {
		assignments[i] = map[string]any{"group_id": g.ID, "course_id": courseID}
	}
	body, _ := json.Marshal(map[string]any{"assignments": assignments})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("schedule round: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Heats []struct {
			ID      int64 `json:"id"`
			GroupID int64 `json:"group_id"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode schedule response: %v", err)
	}
	heatByGroup := make(map[int64]int64, len(resp.Heats))
	for _, h := range resp.Heats {
		heatByGroup[h.GroupID] = h.ID
	}
	return heatByGroup
}

func submitHeatResults(t *testing.T, s *Server, token string, tournamentID, heatID int64, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/heats/%d/results", tournamentID, heatID), bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestSubmitHeatResultsClosesHeatAndCascadesToRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	partial := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if partial.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", partial.Code, partial.Body.String())
	}
	h, err := s.schedule.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h.Status != "scheduled" {
		t.Fatalf("expected heat still scheduled after partial submission, got %s", h.Status)
	}

	remaining := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t2"]: map[string]any{"status": "DNF"},
	})
	if remaining.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", remaining.Code, remaining.Body.String())
	}
	h2, err := s.schedule.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h2.Status != "closed" {
		t.Fatalf("expected heat closed after all results submitted, got %s", h2.Status)
	}
	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusClosed {
		t.Fatalf("expected round closed once its only heat closed, got %s", pr.Status)
	}
}

func TestSubmitHeatResultsRejectsInvalidStatus(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	rec := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"status": "dnf"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for lowercase status, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitHeatResultsInvalidEntryLeavesNothingCommitted(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	rec := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
		ids["t2"]: map[string]any{"status": "not-a-real-status"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	results, err := s.schedule.ListHeatResults(heatID)
	if err != nil {
		t.Fatalf("ListHeatResults: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected zero results committed after a rejected batch, got %d", len(results))
	}
}

func TestSubmitHeatResultsAllowsTimeEntryRole(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, ids := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, organizerToken, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	rec := submitHeatResults(t, s, timeEntryToken, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for time_entry role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitHeatResultsForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, ids := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, organizerToken, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	rec := submitHeatResults(t, s, spectatorToken, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestSubmitHeatResultsBroadcasts(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	conn := dialWS(t, httpServer, tournamentID, token)

	rec := submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("expected a broadcast message: %v", err)
	}
	var evt struct {
		Type   string `json:"type"`
		HeatID int64  `json:"heat_id"`
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("decode broadcast: %v", err)
	}
	if evt.Type != "result_submitted" || evt.HeatID != heatID {
		t.Fatalf("unexpected broadcast event: %+v", evt)
	}
}

func TestSubmitDivisionHeatResultsWorksAndDoesNotReopenRound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	)

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusClosed {
		t.Fatalf("expected round closed before computing divisions, got %s", pr.Status)
	}

	divisionsBody, _ := json.Marshal(map[string]any{"cuts": []map[string]any{{"name": "Final", "size": 2}}})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	var divisionsResp struct {
		Divisions []struct {
			ID int64 `json:"id"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &divisionsResp); err != nil {
		t.Fatalf("decode divisions: %v", err)
	}

	courseID := createTestCourse(t, s, token, tournamentID, "Finals Course", 300)
	scheduleBody, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"division_id": divisionsResp.Divisions[0].ID, "course_id": courseID}},
	})
	scheduleReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/divisions/schedule", tournamentID), bytes.NewReader(scheduleBody))
	scheduleReq.Header.Set("Authorization", "Bearer "+token)
	scheduleRec := httptest.NewRecorder()
	s.ServeHTTP(scheduleRec, scheduleReq)
	var scheduled struct {
		Heats []struct {
			ID int64 `json:"id"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(scheduleRec.Body.Bytes(), &scheduled); err != nil {
		t.Fatalf("decode schedule response: %v", err)
	}
	divisionHeatID := scheduled.Heats[0].ID

	rec := submitHeatResults(t, s, token, tournamentID, divisionHeatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 90.0},
		ids["t2"]: map[string]any{"time_seconds": 95.0},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("submit division heat results: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	h, err := s.schedule.GetHeat(divisionHeatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h.Status != "closed" {
		t.Fatalf("expected division heat closed, got %s", h.Status)
	}

	prAfter, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if prAfter.Status != round.StatusClosed {
		t.Fatalf("expected round to remain closed, got %s", prAfter.Status)
	}
}
