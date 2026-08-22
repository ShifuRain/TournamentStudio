package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
	"tournamentstudio/internal/tournament"
)

type createRoundRequest struct {
	RoundNumber int        `json:"round_number"`
	Groups      [][]string `json:"groups"`
}

type groupResponse struct {
	ID      int64    `json:"id"`
	TeamIDs []string `json:"team_ids"`
}

type roundResponse struct {
	ID          int64           `json:"id"`
	RoundNumber int             `json:"round_number"`
	Status      string          `json:"status"`
	Groups      []groupResponse `json:"groups"`
}

func roundToResponse(pr *round.PrePhaseRound, groups []round.Group) roundResponse {
	gr := make([]groupResponse, 0, len(groups))
	for _, g := range groups {
		gr = append(gr, groupResponse{ID: g.ID, TeamIDs: g.TeamIDs})
	}
	return roundResponse{
		ID:          pr.ID,
		RoundNumber: pr.RoundNumber,
		Status:      string(pr.Status),
		Groups:      gr,
	}
}

func (s *Server) handleCreateRound(w http.ResponseWriter, r *http.Request) {
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

	var req createRoundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RoundNumber < 1 {
		http.Error(w, "round_number must be at least 1", http.StatusBadRequest)
		return
	}
	if len(req.Groups) == 0 {
		http.Error(w, "at least one group is required", http.StatusBadRequest)
		return
	}

	teams, err := s.teams.ListByTournament(tournamentID)
	if err != nil {
		http.Error(w, "could not list teams", http.StatusInternalServerError)
		return
	}
	validTeamIDs := make(map[string]bool, len(teams))
	for _, tm := range teams {
		validTeamIDs[strconv.FormatInt(tm.ID, 10)] = true
	}
	seen := make(map[string]bool)
	for _, group := range req.Groups {
		for _, teamID := range group {
			if !validTeamIDs[teamID] || seen[teamID] {
				http.Error(w, "unknown or duplicate team_id in groups", http.StatusBadRequest)
				return
			}
			seen[teamID] = true
		}
	}

	pr, groups, err := s.rounds.CreateRound(tournamentID, req.RoundNumber, req.Groups)
	if err != nil {
		http.Error(w, "could not create round", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(roundToResponse(pr, groups))
}
