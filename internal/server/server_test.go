package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"tournamentstudio/internal/i18n"
	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/store"
)

// newTestServerWithWebFS is newTestServer's shape, but with an explicit
// webFS -- used by webui_test.go, which needs to control exactly what
// files "exist" to test the SPA-fallback behavior. newTestServer itself
// uses a trivial single-file fstest.MapFS, since no other test in this
// package cares about the embedded frontend's actual content.
func newTestServerWithWebFS(t *testing.T, webFS fs.FS) *Server {
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

	catalog, err := i18n.Load(t.TempDir())
	if err != nil {
		t.Fatalf("load i18n: %v", err)
	}

	return New(s, engine, catalog, webFS)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return newTestServerWithWebFS(t, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	})
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
