package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/round"
)

func createTestRound(t *testing.T, s *Server, token string, tournamentID int64, groups [][]string) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"round_number": 1, "groups": groups})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created round: %v", err)
	}
	return created.ID
}

func TestSubmitResultsClosesRoundWhenComplete(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	partialBody, _ := json.Marshal(map[string]any{"t1": map[string]any{"time_seconds": 124.5}})
	partialReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(partialBody))
	partialReq.Header.Set("Authorization", "Bearer "+token)
	partialRec := httptest.NewRecorder()
	s.ServeHTTP(partialRec, partialReq)
	if partialRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", partialRec.Code, partialRec.Body.String())
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusOpen {
		t.Fatalf("expected round still open after partial submission, got %s", pr.Status)
	}

	remainingBody, _ := json.Marshal(map[string]any{"t2": map[string]any{"status": "DNF"}})
	remainingReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(remainingBody))
	remainingReq.Header.Set("Authorization", "Bearer "+token)
	remainingRec := httptest.NewRecorder()
	s.ServeHTTP(remainingRec, remainingReq)
	if remainingRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", remainingRec.Code, remainingRec.Body.String())
	}

	pr2, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr2.Status != round.StatusClosed {
		t.Fatalf("expected round closed after all results submitted, got %s", pr2.Status)
	}
}

func TestSubmitResultsExtraUnknownTeamIDDoesNotSubstituteForMissingRealTeam(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	// Submit results for all real teams (t1, t2) plus one extra team_id
	// that does not belong to any group in this round. The round should
	// still close: all real teams are covered, and the extra key doesn't
	// block closing.
	allPlusExtraBody, _ := json.Marshal(map[string]any{
		"t1":      map[string]any{"time_seconds": 100.0},
		"t2":      map[string]any{"time_seconds": 110.0},
		"ghostt1": map[string]any{"time_seconds": 999.0},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(allPlusExtraBody))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusClosed {
		t.Fatalf("expected round closed when all real teams covered (plus an extra unknown team_id), got %s", pr.Status)
	}
}

func TestSubmitResultsExtraUnknownTeamIDDoesNotAutoCloseWhenRealTeamMissing(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	// Submit results for only one of the two real teams (t1) plus one
	// bogus extra team_id, so len(results) == 2 by count (matching the
	// number of real teams) but t2, a real team, is still missing. The
	// round must stay open. This is the case the original count-based
	// bug got wrong.
	oneRealPlusExtraBody, _ := json.Marshal(map[string]any{
		"t1":      map[string]any{"time_seconds": 100.0},
		"ghostt1": map[string]any{"time_seconds": 999.0},
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(oneRealPlusExtraBody))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		t.Fatalf("GetRound: %v", err)
	}
	if pr.Status != round.StatusOpen {
		t.Fatalf("expected round to stay open when a real team (t2) is missing a result, even though result count matches team count via a bogus extra team_id; got %s", pr.Status)
	}
}

func TestSubmitResultsAllowsTimeEntryRole(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(map[string]any{"t1": map[string]any{"time_seconds": 100.0}})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for time_entry role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitResultsForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	body, _ := json.Marshal(map[string]any{"t1": map[string]any{"time_seconds": 100.0}})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
