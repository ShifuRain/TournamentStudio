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

func TestScheduleDivisionsCreatesHeats(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2", "t3", "t4"}})
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 120.0},
		"t4", map[string]any{"time_seconds": 130.0},
	)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{{"name": "Gold Final", "size": 2}},
	})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("compute divisions: expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}
	var divisionsResp struct {
		Divisions []struct {
			ID int64 `json:"id"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &divisionsResp); err != nil {
		t.Fatalf("decode divisions: %v", err)
	}
	if len(divisionsResp.Divisions) != 2 {
		t.Fatalf("expected 2 divisions, got %d", len(divisionsResp.Divisions))
	}

	courseID := createTestCourse(t, s, token, tournamentID, "Finals Course", 300)
	scheduleBody, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{
			{"division_id": divisionsResp.Divisions[0].ID, "course_id": courseID},
			{"division_id": divisionsResp.Divisions[1].ID, "course_id": courseID},
		},
		"start_at": "2026-09-01T13:00:00Z",
	})
	scheduleReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/divisions/schedule", tournamentID), bytes.NewReader(scheduleBody))
	scheduleReq.Header.Set("Authorization", "Bearer "+token)
	scheduleRec := httptest.NewRecorder()
	s.ServeHTTP(scheduleRec, scheduleReq)
	if scheduleRec.Code != http.StatusCreated {
		t.Fatalf("schedule divisions: expected 201, got %d: %s", scheduleRec.Code, scheduleRec.Body.String())
	}

	var scheduled struct {
		Heats []struct {
			DivisionID   int64  `json:"division_id"`
			PlannedStart string `json:"planned_start"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(scheduleRec.Body.Bytes(), &scheduled); err != nil {
		t.Fatalf("decode schedule response: %v", err)
	}
	if len(scheduled.Heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(scheduled.Heats))
	}
	first, _ := time.Parse(time.RFC3339, scheduled.Heats[0].PlannedStart)
	second, _ := time.Parse(time.RFC3339, scheduled.Heats[1].PlannedStart)
	if second.Sub(first) != 300*time.Second {
		t.Fatalf("expected division heats 300s apart, got %v", second.Sub(first))
	}
}

func TestScheduleDivisionsRejectsUnknownDivision(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	body, _ := json.Marshal(map[string]any{
		"assignments": []map[string]any{{"division_id": 999, "course_id": courseID}},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/divisions/schedule", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
