package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testWebFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>index</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}
}

func TestWebUIServesIndexAtRoot(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>index</html>" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWebUIServesRealAssetFile(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "console.log('app')" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWebUIFallsBackToIndexForClientSideRoutes(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/tournaments/123", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>index</html>" {
		t.Fatalf("expected index.html fallback, got: %s", rec.Body.String())
	}
}
