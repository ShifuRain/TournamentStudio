package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
)

func TestLoginSuccessAndWhoAmI(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.users.Create("organizer1", "correct-horse", auth.RoleOrganizer); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "organizer1", "password": "correct-horse"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatalf("expected non-empty token")
	}
	if loginResp.Role != "organizer" {
		t.Fatalf("expected role organizer, got %s", loginResp.Role)
	}

	whoReq := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	whoReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	whoRec := httptest.NewRecorder()
	s.ServeHTTP(whoRec, whoReq)
	if whoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", whoRec.Code)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.users.Create("organizer1", "correct-horse", auth.RoleOrganizer); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "organizer1", "password": "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWhoAmIWithoutToken(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
