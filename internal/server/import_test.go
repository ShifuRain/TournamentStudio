package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
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
