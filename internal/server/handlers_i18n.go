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
