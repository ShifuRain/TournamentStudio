package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestNextRoundComputesReseededGroups(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2", "A3", "A4"},
		{"B1", "B2", "B3", "B4"},
	})

	resultsBody, _ := json.Marshal(resultsBodyFor(ids,
		"A1", map[string]any{"time_seconds": 120.0},
		"A2", map[string]any{"time_seconds": 121.0},
		"A3", map[string]any{"time_seconds": 122.0},
		"A4", map[string]any{"time_seconds": 123.0},
		"B1", map[string]any{"time_seconds": 120.0},
		"B2", map[string]any{"time_seconds": 121.0},
		"B3", map[string]any{"time_seconds": 122.0},
		"B4", map[string]any{"time_seconds": 123.0},
	))
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

	wantGroup0 := mapLabels(ids, "A1", "A2", "B3", "B4")
	wantGroup1 := mapLabels(ids, "A3", "A4", "B1", "B2")
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
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	nextReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, roundID), nil)
	nextReq.Header.Set("Authorization", "Bearer "+token)
	nextRec := httptest.NewRecorder()
	s.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", nextRec.Code)
	}
}

// createTestTeamHTTP creates a team via the real POST
// /api/tournaments/{id}/teams endpoint (not the repo directly) and
// returns its real decimal string ID, for the one end-to-end test that
// exercises the actual seam between the round/group layer and the real
// Team domain.
func createTestTeamHTTP(t *testing.T, s *Server, token string, tournamentID int64, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/teams", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create team %q: expected 201, got %d: %s", name, rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created team: %v", err)
	}
	return strconv.FormatInt(created.ID, 10)
}

// TestFullRoundLifecycleWithRealTeams is the first test in this plan to
// exercise the seam between the round/group layer and the real Team
// domain end to end: real teams created via the actual HTTP team-creation
// endpoint (never a synthetic "t1"-style literal), a round built from
// their real IDs, results submitted, and the round advanced via /next.
func TestFullRoundLifecycleWithRealTeams(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	team1 := createTestTeamHTTP(t, s, token, tournamentID, "Rhein Dragons")
	team2 := createTestTeamHTTP(t, s, token, tournamentID, "Kölner DrachenbootClub")

	roundBody, _ := json.Marshal(map[string]any{
		"round_number": 1,
		"groups":       [][]string{{team1, team2}},
	})
	roundReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(roundBody))
	roundReq.Header.Set("Authorization", "Bearer "+token)
	roundRec := httptest.NewRecorder()
	s.ServeHTTP(roundRec, roundReq)
	if roundRec.Code != http.StatusCreated {
		t.Fatalf("create round: expected 201, got %d: %s", roundRec.Code, roundRec.Body.String())
	}
	var createdRound struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(roundRec.Body.Bytes(), &createdRound); err != nil {
		t.Fatalf("decode created round: %v", err)
	}

	resultsBody, _ := json.Marshal(map[string]any{
		team1: map[string]any{"time_seconds": 118.4},
		team2: map[string]any{"time_seconds": 121.9},
	})
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, createdRound.ID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("submit results: expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, createdRound.ID), nil)
	nextReq.Header.Set("Authorization", "Bearer "+token)
	nextRec := httptest.NewRecorder()
	s.ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusCreated {
		t.Fatalf("next round: expected 201, got %d: %s", nextRec.Code, nextRec.Body.String())
	}

	var next struct {
		RoundNumber int `json:"round_number"`
		Groups      []struct {
			TeamIDs []string `json:"team_ids"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(nextRec.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode next round: %v", err)
	}
	if next.RoundNumber != 2 {
		t.Fatalf("expected round number 2, got %d", next.RoundNumber)
	}
	gotTeams := map[string]bool{}
	for _, g := range next.Groups {
		for _, id := range g.TeamIDs {
			gotTeams[id] = true
		}
	}
	if !gotTeams[team1] || !gotTeams[team2] {
		t.Fatalf("expected next round to conserve both real teams %v/%v, got groups %+v", team1, team2, next.Groups)
	}
}

// TestNextRoundIsIdempotentAgainstDoubleSubmission proves that calling
// /next twice in a row on the same closed round (double-click, retry, a
// refreshed tab resubmitting) does not create two distinct rounds both
// numbered N+1 -- the second call must be rejected, and only one round
// N+1 must exist afterward.
func TestNextRoundIsIdempotentAgainstDoubleSubmission(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	resultsBody, _ := json.Marshal(resultsBodyFor(ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	))
	resultsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(resultsBody))
	resultsReq.Header.Set("Authorization", "Bearer "+token)
	resultsRec := httptest.NewRecorder()
	s.ServeHTTP(resultsRec, resultsReq)
	if resultsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resultsRec.Code, resultsRec.Body.String())
	}

	doNext := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/next", tournamentID, roundID), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		return rec
	}

	first := doNext()
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first /next call to succeed with 201, got %d: %s", first.Code, first.Body.String())
	}

	second := doNext()
	if second.Code != http.StatusConflict {
		t.Fatalf("expected second /next call to be rejected with 409, got %d: %s", second.Code, second.Body.String())
	}

	// Confirm exactly one round number 2 exists for this tournament, not
	// two duplicate rounds both numbered 2.
	count, err := s.rounds.CountRounds(tournamentID, 2)
	if err != nil {
		t.Fatalf("CountRounds: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 round numbered 2, got %d", count)
	}
}
