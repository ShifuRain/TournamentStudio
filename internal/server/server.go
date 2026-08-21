package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/store"
)

type Server struct {
	mux      *http.ServeMux
	users    *auth.Repo
	sessions *auth.SessionRepo
}

func New(s *store.Store) *Server {
	srv := &Server{
		mux:      http.NewServeMux(),
		users:    auth.NewRepo(s),
		sessions: auth.NewSessionRepo(s),
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)

	authenticated := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry, auth.RoleSpectator)
	s.mux.Handle("GET /api/whoami", authenticated(http.HandlerFunc(s.handleWhoAmI)))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
