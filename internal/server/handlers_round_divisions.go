package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/round"
)

type computeDivisionsRequest struct {
	Cuts []plugin.Cut `json:"cuts"`
}

func (s *Server) handleComputeDivisions(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	roundID, err := strconv.ParseInt(r.PathValue("round_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid round id", http.StatusBadRequest)
		return
	}

	var req computeDivisionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		if err == round.ErrNotFound {
			http.Error(w, "round not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "round not found", http.StatusNotFound)
		return
	}
	if pr.Status != round.StatusClosed {
		http.Error(w, "round must be closed before computing divisions", http.StatusConflict)
		return
	}

	tour, err := s.tournaments.Get(tournamentID)
	if err != nil {
		http.Error(w, "could not get tournament", http.StatusInternalServerError)
		return
	}
	ttp := s.findTournamentType(tour.TournamentTypeID)
	if ttp == nil {
		http.Error(w, "tournament type plugin not found", http.StatusInternalServerError)
		return
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}
	resultsByTeam := make(map[string]round.Result, len(results))
	for _, res := range results {
		resultsByTeam[res.TeamID] = res
	}

	var allResults []ranking.TeamResult
	for _, g := range groups {
		for _, teamID := range g.TeamIDs {
			res := resultsByTeam[teamID]
			allResults = append(allResults, ranking.TeamResult{
				TeamID:      teamID,
				TimeSeconds: res.TimeSeconds,
				Status:      ranking.Status(res.Status),
			})
		}
	}
	ranked := ranking.Rank(allResults)
	rankedIDs := make([]string, len(ranked))
	for i, res := range ranked {
		rankedIDs[i] = res.TeamID
	}

	divisions, err := ttp.DivisionCuts(rankedIDs, req.Cuts)
	if err != nil {
		http.Error(w, "plugin error computing divisions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"divisions": divisions})
}
