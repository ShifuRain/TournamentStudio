package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/tournament"
)

type createTournamentRequest struct {
	Name             string `json:"name"`
	SportPluginID    string `json:"sport_plugin_id"`
	TournamentTypeID string `json:"tournament_type_id"`
	Language         string `json:"language"`
}

func (s *Server) handleCreateTournament(w http.ResponseWriter, r *http.Request) {
	var req createTournamentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.SportPluginID == "" || req.TournamentTypeID == "" {
		http.Error(w, "name, sport_plugin_id and tournament_type_id are required", http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}

	created, err := s.tournaments.Create(tournament.Tournament{
		Name:             req.Name,
		SportPluginID:    req.SportPluginID,
		TournamentTypeID: req.TournamentTypeID,
		Language:         req.Language,
	})
	if err != nil {
		http.Error(w, "could not create tournament", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) handleListTournaments(w http.ResponseWriter, r *http.Request) {
	list, err := s.tournaments.List()
	if err != nil {
		http.Error(w, "could not list tournaments", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleGetTournament(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	t, err := s.tournaments.Get(id)
	if err != nil {
		if err == tournament.ErrNotFound {
			http.Error(w, "tournament not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get tournament", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}
