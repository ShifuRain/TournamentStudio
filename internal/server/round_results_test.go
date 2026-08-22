package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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

// submitHeatResultsForRound schedules every group of roundID onto a
// single freshly created course (via scheduleTestRound, defined in
// heat_results_test.go), then submits results for each of that
// course's heats via the real per-heat results endpoint, keyed by the
// label -> real-team-ID map createTestRound returns. It's the drop-in
// replacement for the old round-level "POST .../rounds/{id}/results"
// call every test in this package used before this task retired that
// endpoint. pairs is a flat label/entry sequence, e.g. "t1",
// map[string]any{"time_seconds": 100.0}, "t2", map[string]any{"status": "DNF"}.
func submitHeatResultsForRound(t *testing.T, s *Server, token string, tournamentID, roundID int64, ids map[string]string, pairs ...any) {
	t.Helper()

	heatByGroup := scheduleTestRound(t, s, token, tournamentID, roundID)
	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	groupOf := make(map[string]int64, len(groups))
	for _, g := range groups {
		for _, teamID := range g.TeamIDs {
			groupOf[teamID] = g.ID
		}
	}

	byGroup := make(map[int64]map[string]any)
	for i := 0; i < len(pairs); i += 2 {
		label := pairs[i].(string)
		entry := pairs[i+1]
		teamID, ok := ids[label]
		if !ok {
			t.Fatalf("unknown label %q", label)
		}
		groupID, ok := groupOf[teamID]
		if !ok {
			t.Fatalf("team %q (label %q) is not in any of round %d's groups", teamID, label, roundID)
		}
		if byGroup[groupID] == nil {
			byGroup[groupID] = make(map[string]any)
		}
		byGroup[groupID][teamID] = entry
	}

	for groupID, body := range byGroup {
		heatID := heatByGroup[groupID]
		rec := submitHeatResults(t, s, token, tournamentID, heatID, body)
		if rec.Code != http.StatusOK {
			t.Fatalf("submit heat results for group %d: expected 200, got %d: %s", groupID, rec.Code, rec.Body.String())
		}
	}
}
