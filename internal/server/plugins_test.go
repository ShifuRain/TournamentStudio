package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestGetPluginsListsBundledPlugins(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Sports []struct {
			ID string `json:"id"`
		} `json:"sports"`
		TournamentTypes []struct {
			ID string `json:"id"`
		} `json:"tournament_types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	foundSport := false
	for _, sp := range resp.Sports {
		if sp.ID == "dragonboat" {
			foundSport = true
		}
	}
	if !foundSport {
		t.Fatalf("expected dragonboat in sports list, got %v", resp.Sports)
	}

	foundType := false
	for _, tt := range resp.TournamentTypes {
		if tt.ID == "timed-heats-reseeding" {
			foundType = true
		}
	}
	if !foundType {
		t.Fatalf("expected timed-heats-reseeding in tournament_types list, got %v", resp.TournamentTypes)
	}
}
