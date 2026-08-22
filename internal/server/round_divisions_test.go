package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestComputeDivisionsSplitsRankedTeams(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)
	roundID, ids := createTestRound(t, s, token, tournamentID, [][]string{
		{"t1", "t2", "t3", "t4"},
		{"t5", "t6", "t7", "t8"},
	})

	submitHeatResultsForRound(t, s, token, tournamentID, roundID, ids,
		"t1", map[string]any{"time_seconds": 100.0},
		"t2", map[string]any{"time_seconds": 110.0},
		"t3", map[string]any{"time_seconds": 200.0},
		"t4", map[string]any{"time_seconds": 210.0},
		"t5", map[string]any{"time_seconds": 105.0},
		"t6", map[string]any{"time_seconds": 115.0},
		"t7", map[string]any{"time_seconds": 205.0},
		"t8", map[string]any{"time_seconds": 215.0},
	)

	divisionsBody, _ := json.Marshal(map[string]any{
		"cuts": []map[string]any{
			{"name": "Gold Final", "size": 3},
		},
	})
	divisionsReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/rounds/%d/divisions", tournamentID, roundID), bytes.NewReader(divisionsBody))
	divisionsReq.Header.Set("Authorization", "Bearer "+token)
	divisionsRec := httptest.NewRecorder()
	s.ServeHTTP(divisionsRec, divisionsReq)
	if divisionsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", divisionsRec.Code, divisionsRec.Body.String())
	}

	var resp struct {
		Divisions []struct {
			Name    string   `json:"name"`
			TeamIDs []string `json:"team_ids"`
		} `json:"divisions"`
	}
	if err := json.Unmarshal(divisionsRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Divisions) != 2 {
		t.Fatalf("expected 2 divisions (Gold Final + implicit Final), got %d", len(resp.Divisions))
	}

	// The flat ranking (ascending by time, across BOTH groups combined) is:
	// t1(100), t5(105), t2(110), t6(115), t3(200), t7(205), t4(210), t8(215)
	// This interleaves the two groups, so a per-group-then-concatenate bug
	// (Task 10's shape) would produce a different, wrong order.
	wantGold := mapLabels(ids, "t1", "t5", "t2")
	wantFinal := mapLabels(ids, "t6", "t3", "t7", "t4", "t8")

	if resp.Divisions[0].Name != "Gold Final" {
		t.Fatalf("expected first division name %q, got %q", "Gold Final", resp.Divisions[0].Name)
	}
	if !reflect.DeepEqual(resp.Divisions[0].TeamIDs, wantGold) {
		t.Fatalf("expected Gold Final team_ids %v, got %v", wantGold, resp.Divisions[0].TeamIDs)
	}

	if resp.Divisions[1].Name != "Final" {
		t.Fatalf("expected implicit division name %q, got %q", "Final", resp.Divisions[1].Name)
	}
	if !reflect.DeepEqual(resp.Divisions[1].TeamIDs, wantFinal) {
		t.Fatalf("expected Final team_ids %v, got %v", wantFinal, resp.Divisions[1].TeamIDs)
	}
}
