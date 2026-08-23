package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestListRoundsHTTPReturnsGroupsAndEmptyDivisions(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, _ := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}, {"t3", "t4"}})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			ID     int64 `json:"id"`
			Groups []struct {
				ID      int64    `json:"id"`
				TeamIDs []string `json:"team_ids"`
			} `json:"groups"`
			Divisions []struct {
				Name string `json:"name"`
			} `json:"divisions"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(resp.Rounds))
	}
	if resp.Rounds[0].ID != roundID {
		t.Fatalf("expected round id %d, got %d", roundID, resp.Rounds[0].ID)
	}
	if len(resp.Rounds[0].Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Rounds[0].Groups))
	}
	if len(resp.Rounds[0].Divisions) != 0 {
		t.Fatalf("expected 0 divisions before any /divisions call, got %d", len(resp.Rounds[0].Divisions))
	}
}

func TestListRoundsHTTPIncludesDivisionsAfterCut(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{{"t1", "t2"}})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
	)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{{"name": "Gold", "size": 1}},
	})
	divReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divReq.Header.Set("Authorization", "Bearer "+token)
	divRec := httptest.NewRecorder()
	s.ServeHTTP(divRec, divReq)
	if divRec.Code != http.StatusOK {
		t.Fatalf("expected 200 creating divisions, got %d: %s", divRec.Code, divRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []struct {
			Divisions []struct {
				Name string `json:"name"`
			} `json:"divisions"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rounds) != 1 || len(resp.Rounds[0].Divisions) != 2 {
		t.Fatalf("expected 1 round with 2 divisions (Gold + implicit Final), got %+v", resp.Rounds)
	}
}

func TestListRoundsHTTPEmptyForTournamentWithNoRounds(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/rounds", tournamentID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rounds []any `json:"rounds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Rounds) != 0 {
		t.Fatalf("expected 0 rounds, got %d", len(resp.Rounds))
	}
}
