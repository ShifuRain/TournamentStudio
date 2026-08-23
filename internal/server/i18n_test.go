package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetI18nReturnsFlatMap(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/i18n/de", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["next_heat"] != "NÄCHSTER" {
		t.Fatalf("expected German translation, got %q", m["next_heat"])
	}
}

func TestGetI18nRequiresNoAuth(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/i18n/en", nil)
	// Deliberately no Authorization header.
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no auth header, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetI18nLanguagesListsLoadedLanguages(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/i18n", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Languages []string `json:"languages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Languages) != 2 || body.Languages[0] != "de" || body.Languages[1] != "en" {
		t.Fatalf("expected [de en], got %v", body.Languages)
	}
}

func TestGetI18nLanguagesRequiresNoAuth(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/i18n", nil)
	// Deliberately no Authorization header.
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no auth header, got %d: %s", rec.Code, rec.Body.String())
	}
}
