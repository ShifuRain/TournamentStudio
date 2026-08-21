package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })

	engine, err := plugin.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	t.Cleanup(engine.Close)

	return New(s, engine)
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}`+"\n" {
		t.Fatalf("unexpected body: %q", got)
	}
}
