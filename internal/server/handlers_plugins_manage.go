package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tournamentstudio/internal/plugin"
)

// maxPluginUploadBytes bounds a single upload -- plugin sources are small,
// hand-written Lua files; the bundled ones are well under 1 KiB.
const maxPluginUploadBytes = 1 << 20 // 1 MiB

var errInvalidPluginFilename = errors.New("filename must be a bare name ending in .lua")

func (s *Server) handleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPluginUploadBytes)
	if err := r.ParseMultipartForm(maxPluginUploadBytes); err != nil {
		http.Error(w, "invalid multipart upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename, err := sanitizePluginFilename(header.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	source, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read upload", http.StatusBadRequest)
		return
	}

	if err := plugin.Validate(filename, source); err != nil {
		http.Error(w, "invalid plugin: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(s.pluginsDir, 0o755); err != nil {
		http.Error(w, "could not prepare plugins directory", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(s.pluginsDir, filename), source, 0o644); err != nil {
		http.Error(w, "could not save plugin", http.StatusInternalServerError)
		return
	}

	if err := s.reloadPlugins(); err != nil {
		http.Error(w, "plugin saved but reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"filename": filename})
}

func (s *Server) handleDeletePlugin(w http.ResponseWriter, r *http.Request) {
	filename, err := sanitizePluginFilename(r.PathValue("filename"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Bundled plugins are embedded, never written to pluginsDir, so this
	// check alone gives the right 404 for both "unknown filename" and
	// "that's a built-in plugin's name" without needing to consult the
	// live Engine at all.
	path := filepath.Join(s.pluginsDir, filename)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "plugin not found", http.StatusNotFound)
		return
	}
	if err := os.Remove(path); err != nil {
		http.Error(w, "could not delete plugin", http.StatusInternalServerError)
		return
	}

	if err := s.reloadPlugins(); err != nil {
		http.Error(w, "plugin deleted but reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reloadPlugins rebuilds the Engine from scratch (bundled plugins plus
// everything currently in s.pluginsDir) and atomically swaps it in. The
// superseded Engine is deliberately never closed: an in-flight request may
// still hold a *plugin.TournamentTypePlugin obtained from it, and closing
// that plugin's Lua state out from under an in-progress call would race.
// gopher-lua's LState is pure Go with no external OS resource, so the old
// Engine is simply left for garbage collection once nothing references it.
func (s *Server) reloadPlugins() error {
	engine, err := plugin.Load(s.pluginsDir)
	if err != nil {
		return err
	}
	s.plugins.Store(engine)
	return nil
}

// sanitizePluginFilename rejects anything but a bare "name.lua" -- no path
// separators, no "..", so an upload or delete request can never escape
// s.pluginsDir.
func sanitizePluginFilename(name string) (string, error) {
	if name == "" {
		return "", errInvalidPluginFilename
	}
	if filepath.Base(name) != name {
		return "", errInvalidPluginFilename
	}
	if !strings.HasSuffix(name, ".lua") {
		return "", errInvalidPluginFilename
	}
	return name, nil
}
