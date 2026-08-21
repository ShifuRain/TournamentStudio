package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/team"
)

type createTeamRequest struct {
	Name        string            `json:"name"`
	Club        string            `json:"club"`
	ExtraFields map[string]string `json:"extra_fields"`
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req createTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	created, err := s.teams.Create(team.Team{
		TournamentID: tournamentID,
		Name:         req.Name,
		Club:         req.Club,
		ExtraFields:  req.ExtraFields,
	})
	if err != nil {
		http.Error(w, "could not create team", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	list, err := s.teams.ListByTournament(tournamentID)
	if err != nil {
		http.Error(w, "could not list teams", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}
