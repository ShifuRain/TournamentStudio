package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"tournamentstudio/internal/auth"
	"tournamentstudio/internal/i18n"
	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
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
	plugins     *plugin.Engine
	rounds      *round.Repo
	hub         *broadcastHub
	schedule    *schedule.Repo
	i18n        *i18n.Catalog
	webFS       fs.FS
}

func New(s *store.Store, plugins *plugin.Engine, catalog *i18n.Catalog, webFS fs.FS) *Server {
	srv := &Server{
		mux:         http.NewServeMux(),
		users:       auth.NewRepo(s),
		sessions:    auth.NewSessionRepo(s),
		tournaments: tournament.NewRepo(s),
		teams:       team.NewRepo(s),
		plugins:     plugins,
		rounds:      round.NewRepo(s),
		hub:         newBroadcastHub(),
		schedule:    schedule.NewRepo(s),
		i18n:        catalog,
		webFS:       webFS,
	}
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("GET /api/i18n/{lang}", s.handleI18n)

	authenticated := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry, auth.RoleSpectator)
	organizerOnly := s.requireRole(auth.RoleOrganizer)
	resultsWriter := s.requireRole(auth.RoleOrganizer, auth.RoleTimeEntry)

	s.mux.Handle("GET /api/whoami", authenticated(http.HandlerFunc(s.handleWhoAmI)))
	s.mux.Handle("POST /api/logout", authenticated(http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("POST /api/tournaments", organizerOnly(http.HandlerFunc(s.handleCreateTournament)))
	s.mux.Handle("GET /api/tournaments", authenticated(http.HandlerFunc(s.handleListTournaments)))
	s.mux.Handle("GET /api/tournaments/{id}", authenticated(http.HandlerFunc(s.handleGetTournament)))
	s.mux.Handle("POST /api/tournaments/{id}/teams", organizerOnly(http.HandlerFunc(s.handleCreateTeam)))
	s.mux.Handle("GET /api/tournaments/{id}/teams", authenticated(http.HandlerFunc(s.handleListTeams)))
	s.mux.Handle("POST /api/tournaments/{id}/teams/import", organizerOnly(http.HandlerFunc(s.handleImportTeams)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds", organizerOnly(http.HandlerFunc(s.handleCreateRound)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/next", organizerOnly(http.HandlerFunc(s.handleNextRound)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/divisions", organizerOnly(http.HandlerFunc(s.handleComputeDivisions)))
	s.mux.Handle("GET /api/plugins", authenticated(http.HandlerFunc(s.handlePlugins)))
	s.mux.HandleFunc("GET /api/tournaments/{id}/ws", s.handleWebSocket)
	s.mux.Handle("POST /api/tournaments/{id}/courses", organizerOnly(http.HandlerFunc(s.handleCreateCourse)))
	s.mux.Handle("GET /api/tournaments/{id}/courses", authenticated(http.HandlerFunc(s.handleListCourses)))
	s.mux.Handle("PATCH /api/tournaments/{id}/courses/{course_id}", organizerOnly(http.HandlerFunc(s.handleUpdateCourse)))
	s.mux.Handle("POST /api/tournaments/{id}/rounds/{round_id}/schedule", organizerOnly(http.HandlerFunc(s.handleScheduleRound)))
	s.mux.Handle("POST /api/tournaments/{id}/heats/{heat_id}/results", resultsWriter(http.HandlerFunc(s.handleSubmitHeatResults)))
	s.mux.Handle("POST /api/tournaments/{id}/divisions/schedule", organizerOnly(http.HandlerFunc(s.handleScheduleDivisions)))
	s.mux.Handle("PATCH /api/tournaments/{id}/heats/{heat_id}", organizerOnly(http.HandlerFunc(s.handleUpdateHeat)))
	s.mux.Handle("GET /api/tournaments/{id}/schedule", authenticated(http.HandlerFunc(s.handleGetSchedule)))

	s.mux.Handle("/", s.webUIHandler())
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
