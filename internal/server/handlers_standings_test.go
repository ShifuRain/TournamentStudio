package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestGetStandingsRanksGroupByTime(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2", "A3"},
	})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"A1", map[string]any{"time_seconds": 130.5},
		"A2", map[string]any{"time_seconds": 124.11},
		"A3", map[string]any{"status": "DNF"},
	)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/standings", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			ID          int64  `json:"id"`
			RoundNumber int    `json:"round_number"`
			Status      string `json:"status"`
			Standings   []struct {
				GroupID      *int64  `json:"group_id"`
				DivisionID   *int64  `json:"division_id"`
				DivisionName *string `json:"division_name"`
				RankedTeams  []struct {
					Rank        int      `json:"rank"`
					TeamID      string   `json:"team_id"`
					TimeSeconds *float64 `json:"time_seconds"`
					Status      string   `json:"status"`
				} `json:"ranked_teams"`
			} `json:"standings"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode standings response: %v", err)
	}

	if len(resp.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(resp.Rounds))
	}
	rd := resp.Rounds[0]
	if rd.ID != roundID || rd.RoundNumber != 1 {
		t.Fatalf("unexpected round: %+v", rd)
	}
	if len(rd.Standings) != 1 {
		t.Fatalf("expected 1 standings entry (one group), got %d", len(rd.Standings))
	}
	entry := rd.Standings[0]
	if entry.GroupID == nil || entry.DivisionID != nil {
		t.Fatalf("expected a group entry with nil division_id, got %+v", entry)
	}
	if len(entry.RankedTeams) != 3 {
		t.Fatalf("expected 3 ranked teams, got %d", len(entry.RankedTeams))
	}
	wantOrder := mapLabels(ids, "A2", "A1", "A3")
	for i, teamID := range wantOrder {
		if entry.RankedTeams[i].TeamID != teamID {
			t.Fatalf("position %d: expected team %s, got %s", i, teamID, entry.RankedTeams[i].TeamID)
		}
		if entry.RankedTeams[i].Rank != i+1 {
			t.Fatalf("position %d: expected rank %d, got %d", i, i+1, entry.RankedTeams[i].Rank)
		}
	}
	if entry.RankedTeams[2].Status != "DNF" {
		t.Fatalf("expected last-place team to have status DNF, got %q", entry.RankedTeams[2].Status)
	}
}

func TestGetStandingsOmitsTeamsWithoutResults(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"A1", "A2"},
	})

	// Only A1 has a submitted result; A2 hasn't raced yet and must not
	// appear in ranked_teams at all (never sorted to the bottom
	// indistinguishable from a real DNF/DSQ/DNS).
	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"A1", map[string]any{"time_seconds": 100.0},
	)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/standings", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			Standings []struct {
				RankedTeams []struct {
					TeamID string `json:"team_id"`
				} `json:"ranked_teams"`
			} `json:"standings"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode standings response: %v", err)
	}

	rankedTeams := resp.Rounds[0].Standings[0].RankedTeams
	if len(rankedTeams) != 1 {
		t.Fatalf("expected exactly 1 ranked team (only A1 has a result), got %d", len(rankedTeams))
	}
	if rankedTeams[0].TeamID != ids["A1"] {
		t.Fatalf("expected the ranked team to be A1, got %s", rankedTeams[0].TeamID)
	}
}

func TestGetStandingsUsesDivisionsAfterCut(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"t1", "t2", "t3", "t4"},
	})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 200.0},
		"t4", map[string]any{"time_seconds": 210.0},
	)

	divisionsBody := `{"cuts": [{"name": "Gold Final", "size": 2}, {"name": "Silver Final", "size": 2}]}`
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), stringsReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("compute divisions: expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/standings", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			Standings []struct {
				GroupID      *int64  `json:"group_id"`
				DivisionID   *int64  `json:"division_id"`
				DivisionName *string `json:"division_name"`
			} `json:"standings"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode standings response: %v", err)
	}

	if len(resp.Rounds[0].Standings) != 2 {
		t.Fatalf("expected 2 division entries, got %d", len(resp.Rounds[0].Standings))
	}
	for _, entry := range resp.Rounds[0].Standings {
		if entry.GroupID != nil {
			t.Fatalf("expected nil group_id once divisions exist, got %v", *entry.GroupID)
		}
		if entry.DivisionID == nil || entry.DivisionName == nil {
			t.Fatalf("expected division_id and division_name to be set, got %+v", entry)
		}
	}
}

func stringsReader(s string) *strings.Reader {
	return strings.NewReader(s)
}
