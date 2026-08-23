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

func TestWebUIFallsBackToIndexForNestedClientSideRoutes(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/tournaments/1/teams", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "<html>index</html>" {
		t.Fatalf("expected index.html fallback, got: %s", rec.Body.String())
	}
}

// TestUnmatchedAPIPathReturns404 guards against the SPA catch-all ("/")
// swallowing unmatched /api/... paths and returning 200+HTML instead of
// a proper 404 -- a regression this branch's SPA routing introduced.
func TestUnmatchedAPIPathReturns404(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/api/nope", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestWrongMethodOnAPIPathReturns405 guards against the same regression
// for the method-mismatch case: GET on a POST-only endpoint like
// /api/login must 405, not fall through to the SPA catch-all.
func TestWrongMethodOnAPIPathReturns405(t *testing.T) {
	s := newTestServerWithWebFS(t, testWebFS())

	req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
	if allow := rec.Header().Get("Allow"); allow != "POST" {
		t.Fatalf("expected Allow: POST, got %q", allow)
	}
}
