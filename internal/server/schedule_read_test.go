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

func TestGetScheduleReturnsEffectiveTimeAndResults(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})
	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	var heatID int64
	for _, id := range heatByGroup {
		heatID = id
	}

	scheduleReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/schedule", tournamentID), nil)
	scheduleReq.Header.Set("Authorization", "Bearer "+token)
	scheduleRec := httptest.NewRecorder()
	s.ServeHTTP(scheduleRec, scheduleReq)

	var before struct {
		Heats []struct {
			ID             int64  `json:"id"`
			PlannedStart   string `json:"planned_start"`
			EffectiveStart string `json:"effective_start"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(scheduleRec.Body.Bytes(), &before); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(before.Heats) != 1 {
		t.Fatalf("expected 1 heat, got %d", len(before.Heats))
	}
	if before.Heats[0].PlannedStart != before.Heats[0].EffectiveStart {
		t.Fatalf("expected effective_start to equal planned_start with a zero delay offset")
	}

	// Nudge the course's delay offset, confirm effective_start shifts.
	getHeatResp, err := s.schedule.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	courseID := getHeatResp.CourseID
	patchBody, _ := json.Marshal(map[string]any{"delay_offset_seconds": 600})
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/tournaments/%d/courses/%d", tournamentID, courseID), bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchRec := httptest.NewRecorder()
	s.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch course: expected 200, got %d: %s", patchRec.Code, patchRec.Body.String())
	}

	submitHeatResults(t, s, token, tournamentID, heatID, map[string]any{
		ids["t1"]: map[string]any{"time_seconds": 100.0},
	})

	afterReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/schedule", tournamentID), nil)
	afterReq.Header.Set("Authorization", "Bearer "+token)
	afterRec := httptest.NewRecorder()
	s.ServeHTTP(afterRec, afterReq)

	var after struct {
		Heats []struct {
			PlannedStart   string `json:"planned_start"`
			EffectiveStart string `json:"effective_start"`
			Results        []struct {
				TeamID      string   `json:"team_id"`
				TimeSeconds *float64 `json:"time_seconds"`
			} `json:"results"`
		} `json:"heats"`
	}
	if err := json.Unmarshal(afterRec.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode: %v", err)
	}
	planned, _ := time.Parse(time.RFC3339, after.Heats[0].PlannedStart)
	effective, _ := time.Parse(time.RFC3339, after.Heats[0].EffectiveStart)
	if effective.Sub(planned) != 600*time.Second {
		t.Fatalf("expected effective_start 600s after planned_start, got %v", effective.Sub(planned))
	}
	if len(after.Heats[0].Results) != 1 || after.Heats[0].Results[0].TeamID != ids["t1"] {
		t.Fatalf("expected 1 result for t1, got %+v", after.Heats[0].Results)
	}
}

func TestGetScheduleAllowsSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/schedule", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
