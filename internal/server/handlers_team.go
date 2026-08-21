package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"tournamentstudio/internal/team"
	"tournamentstudio/internal/tournament"
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

	if _, err := s.tournaments.Get(tournamentID); err != nil {
		if err == tournament.ErrNotFound {
			http.Error(w, "tournament not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not look up tournament", http.StatusInternalServerError)
		return
	}

	var req createTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	club := strings.TrimSpace(req.Club)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	created, err := s.teams.Create(team.Team{
		TournamentID: tournamentID,
		Name:         name,
		Club:         club,
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
