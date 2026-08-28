package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/ranking"
)

func newPluginUploadRequest(t *testing.T, token, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/plugins", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func getPluginsCatalog(t *testing.T, s *Server, token string) struct {
	Sports []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	} `json:"sports"`
} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/plugins: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Sports []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"sports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode plugins response: %v", err)
	}
	return resp
}

const validSportPluginLua = `
return {
  id = "extra-sport",
  display_name = "Extra Sport",
  compatible_tournament_types = {"timed-heats-reseeding"},
}
`

func TestUploadPluginAddsToCatalog(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := newPluginUploadRequest(t, token, "extra-sport.lua", []byte(validSportPluginLua))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	catalog := getPluginsCatalog(t, s, token)
	var found bool
	for _, sp := range catalog.Sports {
		if sp.ID == "extra-sport" {
			found = true
			if sp.Source != "extra-sport.lua" {
				t.Fatalf("expected source to be the filename, got %q", sp.Source)
			}
		}
	}
	if !found {
		t.Fatal("expected extra-sport to appear in the catalog after upload")
	}
}

func TestUploadPluginRejectsInvalidLua(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := newPluginUploadRequest(t, token, "broken.lua", []byte("not valid lua {{{"))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	catalog := getPluginsCatalog(t, s, token)
	for _, sp := range catalog.Sports {
		if sp.ID == "broken" {
			t.Fatal("a rejected upload must not appear in the catalog")
		}
	}
}

func TestUploadPluginForbiddenForNonOrganizer(t *testing.T) {
	s := newTestServer(t)
	loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	spectatorToken := loginAs(t, s, "spectator1", "pw", auth.RoleSpectator)

	req := newPluginUploadRequest(t, spectatorToken, "extra-sport.lua", []byte(validSportPluginLua))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletePluginRemovesExternalPlugin(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	uploadReq := newPluginUploadRequest(t, token, "extra-sport.lua", []byte(validSportPluginLua))
	uploadRec := httptest.NewRecorder()
	s.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/plugins/extra-sport.lua", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRec := httptest.NewRecorder()
	s.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", delRec.Code, delRec.Body.String())
	}

	catalog := getPluginsCatalog(t, s, token)
	for _, sp := range catalog.Sports {
		if sp.ID == "extra-sport" {
			t.Fatal("expected extra-sport to be gone from the catalog after delete")
		}
	}
}

func TestDeletePluginBundledReturnsNotFound(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/dragonboat.lua", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a bundled plugin's filename, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletePluginForbiddenForNonOrganizer(t *testing.T) {
	s := newTestServer(t)
	organizerToken := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)
	timeEntryToken := loginAs(t, s, "timeentry1", "pw", auth.RoleTimeEntry)

	uploadReq := newPluginUploadRequest(t, organizerToken, "extra-sport.lua", []byte(validSportPluginLua))
	uploadRec := httptest.NewRecorder()
	s.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/extra-sport.lua", nil)
	req.Header.Set("Authorization", "Bearer "+timeEntryToken)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUploadPluginFilenameIsSanitizedByStdlib documents (rather than the
// naively-expected 400) what actually happens when a "../escaped.lua"
// filename is uploaded: mime/multipart's Part.FileName -- which is what
// populates FileHeader.Filename behind r.FormFile -- already passes the
// value through filepath.Base before handleUploadPlugin ever sees it (see
// https://pkg.go.dev/mime/multipart#Part.FileName, "the filename is passed
// through filepath.Base"). So the attacker-controlled value our handler
// actually receives is the bare, harmless "escaped.lua", and the upload
// succeeds. sanitizePluginFilename's own path-traversal guard is still
// real and still load-bearing -- it protects the DELETE endpoint, whose
// {filename} path segment is NOT stdlib-sanitized this way (see
// TestDeletePluginRejectsPathTraversalFilename) -- and is exercised
// directly, independent of any HTTP transport, by
// TestSanitizePluginFilenameRejectsPathTraversal below.
func TestUploadPluginFilenameIsSanitizedByStdlib(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := newPluginUploadRequest(t, token, "../escaped.lua", []byte(validSportPluginLua))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (stdlib already stripped the path component), got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Filename != "escaped.lua" {
		t.Fatalf("expected the stdlib-sanitized filename %q, got %q", "escaped.lua", resp.Filename)
	}
}

// TestSanitizePluginFilenameRejectsPathTraversal tests sanitizePluginFilename
// directly, independent of any HTTP transport's own quirks (see
// TestUploadPluginFilenameIsSanitizedByStdlib for why the upload endpoint
// itself can never observe an unsanitized value here).
func TestSanitizePluginFilenameRejectsPathTraversal(t *testing.T) {
	cases := []string{
		"../escaped.lua",
		"../../etc/passwd.lua",
		"sub/dir.lua",
		"/etc/passwd.lua",
		"..",
		"",
	}
	for _, name := range cases {
		if _, err := sanitizePluginFilename(name); err == nil {
			t.Errorf("expected sanitizePluginFilename(%q) to be rejected", name)
		}
	}
}

// TestDeletePluginRejectsPathTraversalFilename covers the real,
// HTTP-reachable path-traversal surface: unlike the upload endpoint's
// multipart filename, the DELETE endpoint's {filename} path segment is not
// run through filepath.Base by net/http. A literal ".." path segment (e.g.
// "/api/plugins/../escaped.lua") is cleaned by net/http's own redirect
// before a handler ever runs, but a URL-encoded slash inside the single
// {filename} segment is not: net/http decodes "..%2Fescaped.lua" to
// "../escaped.lua" and hands that straight to r.PathValue("filename"),
// which handleDeletePlugin passes to sanitizePluginFilename. Confirmed by
// direct experiment against net/http's ServeMux before writing this test.
func TestDeletePluginRejectsPathTraversalFilename(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	req := httptest.NewRequest(http.MethodDelete, "/api/plugins/..%2Fescaped.lua", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a path-traversal filename, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReloadDoesNotInvalidateInFlightTournamentTypePlugin(t *testing.T) {
	s := newTestServer(t)
	token := loginAs(t, s, "organizer1", "pw", auth.RoleOrganizer)

	ttp := s.findTournamentType("timed-heats-reseeding")
	if ttp == nil {
		t.Fatal("expected bundled timed-heats-reseeding tournament type plugin")
	}

	// Trigger a reload out from under the reference just obtained.
	uploadReq := newPluginUploadRequest(t, token, "extra-sport.lua", []byte(validSportPluginLua))
	uploadRec := httptest.NewRecorder()
	s.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", uploadRec.Code, uploadRec.Body.String())
	}

	// The reference obtained before the reload must still work -- its Lua
	// state was never closed out from under this call.
	timeA, timeB := 100.0, 200.0
	if _, err := ttp.NextRoundGroups([][]ranking.TeamResult{
		{{TeamID: "1", TimeSeconds: &timeA}, {TeamID: "2", TimeSeconds: &timeB}},
	}); err != nil {
		t.Fatalf("expected the pre-reload plugin reference to keep working, got: %v", err)
	}
}
