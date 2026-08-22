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
)

// createTestCourse creates a course via the real HTTP endpoint and
// returns its ID, for reuse by every later task's tests that need a
// course to schedule heats onto.
func createTestCourse(t *testing.T, s *Server, token string, tournamentID int64, name string, heatIntervalSeconds int) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "heat_interval_seconds": heatIntervalSeconds})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created course: %v", err)
	}
	return created.ID
}

func TestCreateAndListCoursesHTTP(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	createTestCourse(t, s, token, tournamentID, "Course A", 300)
	createTestCourse(t, s, token, tournamentID, "Course B", 240)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Courses []struct {
			Name string `json:"name"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Courses) != 2 {
		t.Fatalf("expected 2 courses, got %d", len(resp.Courses))
	}
}

func TestCreateCourseRejectsMissingName(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	body, _ := json.Marshal(map[string]any{"heat_interval_seconds": 300})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateCourseForbiddenForTimeEntry(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{"name": "Course A", "heat_interval_seconds": 300})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/courses", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestUpdateCourseDelayOffsetBroadcasts(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	courseID := createTestCourse(t, s, token, tournamentID, "Course A", 300)

	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	conn := dialWS(t, httpServer, tournamentID, token)

	body, _ := json.Marshal(map[string]any{"delay_offset_seconds": 900})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/courses/%d", tournamentID, courseID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated struct {
		DelayOffsetSeconds int `json:"delay_offset_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.DelayOffsetSeconds != 900 {
		t.Fatalf("expected 900, got %d", updated.DelayOffsetSeconds)
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("expected a broadcast message: %v", err)
	}
	var evt struct {
		Type               string `json:"type"`
		CourseID           int64  `json:"course_id"`
		DelayOffsetSeconds int    `json:"delay_offset_seconds"`
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("decode broadcast: %v", err)
	}
	if evt.Type != "delay_offset_changed" || evt.CourseID != courseID || evt.DelayOffsetSeconds != 900 {
		t.Fatalf("unexpected broadcast event: %+v", evt)
	}
}

func TestUpdateCourseNotFoundForWrongTournament(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentA := createTestTournament(t, s, token)
	tournamentB := createTestTournament(t, s, token)
	courseID := createTestCourse(t, s, token, tournamentA, "Course A", 300)

	body, _ := json.Marshal(map[string]any{"name": "Hijacked"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/courses/%d", tournamentB, courseID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
