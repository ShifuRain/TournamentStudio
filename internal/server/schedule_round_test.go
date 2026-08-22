package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tournamentstudio/internal/auth"
)

func TestScheduleRoundCreatesHeatsForEveryGroup(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"A1", "A2"}, {"B1", "B2"}})
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{
			{"group_id": groups[0].ID, "course_id": courseID},
			{"group_id": groups[1].ID, "course_id": courseID},
		},
		"start_at": "2026-09-01T09:00:00Z",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Heats []struct {
			ID           int64  `json:"id"`
			GroupID      int64  `json:"group_id"`
			PlannedStart string `json:"planned_start"`
			Status       string `json:"status"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(resp.Heats))
	}
	first, err := time.Parse(time.RFC3339, resp.Heats[0].PlannedStart)
	if err != nil {
		t.Fatalf("parse first planned_start: %v", err)
	}
	second, err := time.Parse(time.RFC3339, resp.Heats[1].PlannedStart)
	if err != nil {
		t.Fatalf("parse second planned_start: %v", err)
	}
	if second.Sub(first) != 300*time.Second {
		t.Fatalf("expected heats 300s apart, got %v apart", second.Sub(first))
	}
	if resp.Heats[0].Status != "scheduled" {
		t.Fatalf("expected status scheduled, got %q", resp.Heats[0].Status)
	}
}

func TestScheduleRoundRejectsIncompleteAssignments(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"A1"}, {"B1"}})
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"group_id": groups[0].ID, "course_id": courseID}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestScheduleRoundRejectsDoubleScheduling(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"A1"}})
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"group_id": groups[0].ID, "course_id": courseID}},
	})

	req1 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req1.Header.Set("Authorization", "Bearer "+token)
	rec1 := httptest.NewRecorder()
	s.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected first schedule to succeed with 201, got %d: %s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected second schedule to be rejected with 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestScheduleRoundForbiddenForTimeEntry(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, _ := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"A1"}})
	courseID := createTestCourse(t, s, organizerToken, tournamentID, "Course A", 300)
	groups, _ := s.rounds.ListGroups(roundID)

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"group_id": groups[0].ID, "course_id": courseID}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/schedule", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
