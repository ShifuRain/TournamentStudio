package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.i18n.Strings(lang))
}

// handleI18nLanguages lists the languages currently loaded into the
// catalog -- the bundled ones plus any drop-in language a deployment
// added via TOURNAMENTSTUDIO_LANGUAGES -- so the frontend can discover
// them instead of relying on a hardcoded list.
func (s *Server) handleI18nLanguages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"languages": s.i18n.Languages(),
	})
}
