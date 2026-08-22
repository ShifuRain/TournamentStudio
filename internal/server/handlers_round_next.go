package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/ranking"
)

func (s *Server) handleNextRound(w http.ResponseWriter, r *http.Request) {
	ctx, ok := s.loadClosedRoundContext(w, r)
	if !ok {
		return
	}

	nextRoundNumber := ctx.round.RoundNumber + 1
	exists, err := s.rounds.RoundExists(ctx.tournamentID, nextRoundNumber)
	if err != nil {
		http.Error(w, "could not check for existing round", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "the next round has already been created", http.StatusConflict)
		return
	}

	rankedGroups := make([][]ranking.TeamResult, 0, len(ctx.groups))
	inputTeamIDs := make(map[string]int)
	for _, g := range ctx.groups {
		unranked := make([]ranking.TeamResult, 0, len(g.TeamIDs))
		for _, teamID := range g.TeamIDs {
			res := ctx.resultsByTeam[teamID]
			unranked = append(unranked, ranking.TeamResult{
				TeamID:      teamID,
				TimeSeconds: res.TimeSeconds,
				Status:      ranking.Status(res.Status),
			})
			inputTeamIDs[teamID]++
		}
		rankedGroups = append(rankedGroups, ranking.Rank(unranked))
	}

	nextGroupTeamIDs, err := ctx.ttp.NextRoundGroups(rankedGroups)
	if err != nil {
		http.Error(w, "plugin error computing next round: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// The plugin must return exactly the same set of teams it was given
	// (spec §5's team-conservation invariant) -- verify before persisting
	// anything, so a buggy or malicious plugin can't silently drop, add,
	// or duplicate teams in a way nothing downstream would ever check.
	outputTeamIDs := make(map[string]int)
	outputCount := 0
	for _, group := range nextGroupTeamIDs {
		for _, teamID := range group {
			outputTeamIDs[teamID]++
			outputCount++
		}
	}
	inputCount := 0
	for _, n := range inputTeamIDs {
		inputCount += n
	}
	conserved := outputCount == inputCount
	if conserved {
		for teamID, n := range inputTeamIDs {
			if outputTeamIDs[teamID] != n {
				conserved = false
				break
			}
		}
	}
	if conserved {
		for teamID := range outputTeamIDs {
			if _, ok := inputTeamIDs[teamID]; !ok {
				conserved = false
				break
			}
		}
	}
	if !conserved {
		http.Error(w, "plugin violated team conservation invariant", http.StatusInternalServerError)
		return
	}

	nextPR, nextGroups, err := s.rounds.CreateRound(ctx.tournamentID, nextRoundNumber, nextGroupTeamIDs)
	if err != nil {
		http.Error(w, "could not create next round", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(roundToResponse(nextPR, nextGroups))
}
