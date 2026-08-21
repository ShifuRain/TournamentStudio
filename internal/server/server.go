package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/store"
	"tournamentstudio/internal/team"
	"tournamentstudio/internal/tournament"
)

type Server struct {
	mux         *http.ServeMux
	users       *auth.Repo
	sessions    *auth.SessionRepo
	tournaments *tournament.Repo
	teams       *team.Repo
}

func New(s *store.Store) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)

	authenticated := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry, auth.RoleSpectator)
	organizerOnly := s.requireRole(auth.RoleOrganizer)

	s.mux.Handle("GET /api/whoami", authenticated(http.HandlerFunc(s.handleWhoAmI)))
	s.mux.Handle("POST /api/tournaments", organizerOnly(http.HandlerFunc(s.handleCreateTournament)))
	s.mux.Handle("GET /api/tournaments", authenticated(http.HandlerFunc(s.handleListTournaments)))
	s.mux.Handle("GET /api/tournaments/{id}", authenticated(http.HandlerFunc(s.handleGetTournament)))
	s.mux.Handle("POST /api/tournaments/{id}/teams", organizerOnly(http.HandlerFunc(s.handleCreateTeam)))
	s.mux.Handle("GET /api/tournaments/{id}/teams", authenticated(http.HandlerFunc(s.handleListTeams)))
	s.mux.Handle("POST /api/tournaments/{id}/teams/import", organizerOnly(http.HandlerFunc(s.handleImportTeams)))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
