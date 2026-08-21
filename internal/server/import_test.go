package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/store"
)

func TestImportCSVTeamsEndToEnd(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "teams.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write([]byte("name,club\nMöwe RC Kiel,Möwe Ruderclub e.V.\nWassermann Berlin,\n"))
	mw.Close()

	importReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/teams/import", tournamentID), &body)
	importReq.Header.Set("Authorization", "Bearer "+token)
	importReq.Header.Set("Content-Type", mw.FormDataContentType())
	importRec := httptest.NewRecorder()
	s.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", importRec.Code, importRec.Body.String())
	}

	var importResp struct {
		Imported int `json:"imported"`
	}
	if err := json.Unmarshal(importRec.Body.Bytes(), &importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if importResp.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d", importResp.Imported)
	}

	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/teams", tournamentID), nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)

	var teams []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &teams); err != nil {
		t.Fatalf("decode teams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}

// TestImportTeamsPartialPersistenceFailure proves that when one row's
// s.teams.Create call fails after validation has already passed, the
// handler does not abort with a bare 500: it records the failure as an
// additional problem, keeps the rows that did persist, and always returns
// the accumulated imported count and problems list.
//
// To trigger a genuine (not mocked) persistence failure without touching
// team.Repo or the store migrations, this test opens its own store handle
// and adds a unique index on teams(tournament_id, name) purely from the
// test side, then seeds a team with a name that a CSV row will collide
// with. That collision makes s.teams.Create fail for real on that one row.
func TestImportTeamsPartialPersistenceFailure(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.DB.Close() })
	if _, err := st.DB.Exec(`CREATE UNIQUE INDEX idx_teams_test_unique_name ON teams(tournament_id, name)`); err != nil {
		t.Fatalf("create unique index: %v", err)
	}

	s := New(st)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	tournamentID := createTestTournament(t, s, token)

	// Seed a team that collides with one of the CSV rows below, so that
	// row's Create call fails on the unique index while the other row
	// succeeds.
	seedBody, _ := json.Marshal(map[string]any{"name": "Wassermann Berlin", "club": ""})
	seedReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/teams", tournamentID), bytes.NewReader(seedBody))
	seedReq.Header.Set("Authorization", "Bearer "+token)
	seedRec := httptest.NewRecorder()
	s.ServeHTTP(seedRec, seedReq)
	if seedRec.Code != http.StatusCreated {
		t.Fatalf("seed team: expected 201, got %d: %s", seedRec.Code, seedRec.Body.String())
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "teams.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write([]byte("name,club\nMöwe RC Kiel,Möwe Ruderclub e.V.\nWassermann Berlin,\n"))
	mw.Close()

	importReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tournaments/%d/teams/import", tournamentID), &body)
	importReq.Header.Set("Authorization", "Bearer "+token)
	importReq.Header.Set("Content-Type", mw.FormDataContentType())
	importRec := httptest.NewRecorder()
	s.ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with a partial persistence failure, got %d: %s", importRec.Code, importRec.Body.String())
	}

	var importResp struct {
		Imported int              `json:"imported"`
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal(importRec.Body.Bytes(), &importResp); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if importResp.Imported != 1 {
		t.Fatalf("expected 1 imported (the non-colliding row), got %d", importResp.Imported)
	}
	if len(importResp.Problems) != 1 {
		t.Fatalf("expected 1 problem for the colliding row, got %d: %+v", len(importResp.Problems), importResp.Problems)
	}

	listReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tournaments/%d/teams", tournamentID), nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	s.ServeHTTP(listRec, listReq)

	var teams []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &teams); err != nil {
		t.Fatalf("decode teams: %v", err)
	}
	// 1 seeded team + 1 successfully imported team = 2; the colliding row
	// must not have been silently lost from the response, but it also
	// must not appear twice in the list.
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams (seed + successfully imported row), got %d", len(teams))
	}
}
