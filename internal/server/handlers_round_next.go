package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/round"
)

func (s *Server) findTournamentType(id string) *plugin.TournamentTypePlugin {
	for _, ttp := range s.plugins.TournamentTypes() {
		if ttp.ID == id {
			return ttp
		}
	}
	return nil
}

func (s *Server) handleNextRound(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "round must be closed before computing the next round", http.StatusConflict)
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

	rankedGroups := make([][]ranking.TeamResult, 0, len(groups))
	for _, g := range groups {
		unranked := make([]ranking.TeamResult, 0, len(g.TeamIDs))
		for _, teamID := range g.TeamIDs {
			res := resultsByTeam[teamID]
			unranked = append(unranked, ranking.TeamResult{
				TeamID:      teamID,
				TimeSeconds: res.TimeSeconds,
				Status:      ranking.Status(res.Status),
			})
		}
		rankedGroups = append(rankedGroups, ranking.Rank(unranked))
	}

	nextGroupTeamIDs, err := ttp.NextRoundGroups(rankedGroups)
	if err != nil {
		http.Error(w, "plugin error computing next round: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nextPR, nextGroups, err := s.rounds.CreateRound(tournamentID, pr.RoundNumber+1, nextGroupTeamIDs)
	if err != nil {
		http.Error(w, "could not create next round", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(roundToResponse(nextPR, nextGroups))
}
