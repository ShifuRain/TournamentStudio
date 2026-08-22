package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/round"
	"tournamentstudio/internal/team"
)

// createRealTeams creates one real team per label (directly via the team
// repo, bypassing HTTP for speed -- what matters for handleCreateRound's
// validation is that a row exists in the teams table, not how it got
// there) and returns a map from label to that team's real decimal string
// ID. Tests throughout this package use short synthetic labels like "t1"
// or "A1"; this lets them keep using those labels as map keys while the
// actual team_ids sent over the wire are real IDs handleCreateRound will
// accept.
func createRealTeams(t *testing.T, s *Server, tournamentID int64, labels []string) map[string]string {
	t.Helper()
	ids := make(map[string]string, len(labels))
	for _, label := range labels {
		created, err := s.teams.Create(team.Team{TournamentID: tournamentID, Name: label})
		if err != nil {
			t.Fatalf("create real team %q: %v", label, err)
		}
		ids[label] = strconv.FormatInt(created.ID, 10)
	}
	return ids
}

// uniqueLabels returns the distinct team-id labels appearing in groups, in
// first-seen order.
func uniqueLabels(groups [][]string) []string {
	var labels []string
	seen := make(map[string]bool)
	for _, g := range groups {
		for _, label := range g {
			if !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}
	return labels
}

// createTestRound creates one real team per distinct label used in groups,
// then creates a round whose group team_ids are those teams' real decimal
// IDs (handleCreateRound validates team_ids against real teams, so a
// synthetic label like "t1" would otherwise be rejected). It returns the
// created round's ID and the label -> real-ID map, so callers can keep
// writing test bodies and assertions in terms of the original short
// labels.
func createTestRound(t *testing.T, s *Server, token string, tournamentID int64, groups [][]string) (int64, map[string]string) {
	t.Helper()
	ids := createRealTeams(t, s, tournamentID, uniqueLabels(groups))

	realGroups := make([][]string, len(groups))
	for i, g := range groups {
		rg := make([]string, len(g))
		for j, label := range g {
			rg[j] = ids[label]
		}
		realGroups[i] = rg
	}

	body, _ := json.Marshal(map[string]any{"round_number": 1, "groups": realGroups})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created round: %v (status %d, body %s)", err, rec.Code, rec.Body.String())
	}
	return created.ID, ids
}

// resultsBodyFor builds a submit-results request body keyed by the real
// team IDs corresponding to the given labels, e.g.
// resultsBodyFor(ids, "t1", map[string]any{"time_seconds": 100.0}, ...).
func resultsBodyFor(ids map[string]string, pairs ...any) map[string]any {
	body := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		label := pairs[i].(string)
		key, ok := ids[label]
		if !ok {
			key = label // not a mapped label (e.g. a deliberately bogus/unknown id) -- use as-is
		}
		body[key] = pairs[i+1]
	}
	return body
}

// mapLabels translates a sequence of short labels (e.g. "t1") into their
// real decimal team IDs per ids, for building expected values to compare
// against real API responses.
func mapLabels(ids map[string]string, labels ...string) []string {
	out := make([]string, len(labels))
	for i, label := range labels {
		out[i] = ids[label]
	}
	return out
}

func TestSubmitResultsClosesRoundWhenComplete(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	partialBody, _ := json.Marshal(resultsBodyFor(ids, "t1", map[string]any{"time_seconds": 124.5}))
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

	remainingBody, _ := json.Marshal(resultsBodyFor(ids, "t2", map[string]any{"status": "DNF"}))
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
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	// Submit results for all real teams (t1, t2) plus one extra team_id
	// that does not belong to any group in this round. The round should
	// still close: all real teams are covered, and the extra key doesn't
	// block closing.
	allPlusExtraBody, _ := json.Marshal(resultsBodyFor(ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"ghostt1", map[string]any{"time_seconds": 999.0},
	))
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
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	// Submit results for only one of the two real teams (t1) plus one
	// bogus extra team_id, so len(results) == 2 by count (matching the
	// number of real teams) but t2, a real team, is still missing. The
	// round must stay open. This is the case the original count-based
	// bug got wrong.
	oneRealPlusExtraBody, _ := json.Marshal(resultsBodyFor(ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"ghostt1", map[string]any{"time_seconds": 999.0},
	))
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
	roundID, ids := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})

	timeEntryToken := loginAs(t, s, "timekeeper1", "pw", auth.RoleTimeEntry)
	body, _ := json.Marshal(resultsBodyFor(ids, "t1", map[string]any{"time_seconds": 100.0}))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for time_entry role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitResultsRejectsInvalidStatus(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	body, _ := json.Marshal(resultsBodyFor(ids, "t1", map[string]any{"status": "dnf"}))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for lowercase status, got %d: %s", rec.Code, rec.Body.String())
	}

	body2, _ := json.Marshal(resultsBodyFor(ids, "t2", map[string]any{"status": "MAYBE"}))
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unrecognized status, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestSubmitResultsInvalidEntryLeavesNothingCommitted(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	// t1 is valid, t2 has an invalid status. Regardless of map iteration
	// order, the whole request must be rejected with nothing written.
	body, _ := json.Marshal(resultsBodyFor(ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"status": "not-a-real-status"},
	))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		t.Fatalf("ListResults: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected zero results committed after a rejected batch, got %d: %+v", len(results), results)
	}
}

func TestSubmitResultsForbiddenForSpectator(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, organizerToken)
	roundID, ids := createTestRound(t, s, organizerToken, tournamentID, [][]string{{"t1"}})

	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)
	body, _ := json.Marshal(resultsBodyFor(ids, "t1", map[string]any{"time_seconds": 100.0}))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/results", tournamentID, roundID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+spectatorToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
