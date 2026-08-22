package server

import (
	"encoding/json"
	"net/http"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/ranking"
)

type computeDivisionsRequest struct {
	Cuts []plugin.Cut `json:"cuts"`
}

func (s *Server) handleComputeDivisions(w http.ResponseWriter, r *http.Request) {
	var req computeDivisionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, ok := s.loadClosedRoundContext(w, r)
	if !ok {
		return
	}

	var allResults []ranking.TeamResult
	for _, g := range ctx.groups {
		for _, teamID := range g.TeamIDs {
			res := ctx.resultsByTeam[teamID]
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

	divisions, err := ctx.ttp.DivisionCuts(rankedIDs, req.Cuts)
	if err != nil {
		http.Error(w, "plugin error computing divisions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"divisions": divisions})
}
