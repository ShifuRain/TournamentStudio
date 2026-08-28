package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/ranking"
	"tournamentstudio/internal/schedule"
)

type rankedTeamResponse struct {
	Rank        int      `json:"rank"`
	TeamID      string   `json:"team_id"`
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}

type standingsEntryResponse struct {
	GroupID      *int64                `json:"group_id"`
	DivisionID   *int64                `json:"division_id"`
	DivisionName *string               `json:"division_name"`
	RankedTeams  []rankedTeamResponse  `json:"ranked_teams"`
}

type standingsRoundResponse struct {
	ID          int64                     `json:"id"`
	RoundNumber int                       `json:"round_number"`
	Status      string                    `json:"status"`
	Standings   []standingsEntryResponse  `json:"standings"`
}

// handleGetStandings returns every round's groups -- or, once a round has
// divisions, its divisions instead -- with teams ranked fastest-first via
// ranking.Rank(). Unlike handleComputeDivisions (which only ever runs on a
// closed round, so it can safely treat "no result yet" as the worst
// possible outcome), this endpoint must also render correctly for an open,
// in-progress round: a team with no entry in that round's results is
// omitted from ranked_teams entirely, not sorted to the bottom
// indistinguishable from a real DNF/DSQ/DNS.
func (s *Server) handleGetStandings(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	rounds, err := s.rounds.ListRounds(tournamentID)
	if err != nil {
		http.Error(w, "could not list rounds", http.StatusInternalServerError)
		return
	}

	resp := make([]standingsRoundResponse, 0, len(rounds))
	for _, rd := range rounds {
		results, err := s.schedule.ListResultsForRound(rd.ID)
		if err != nil {
			http.Error(w, "could not list results", http.StatusInternalServerError)
			return
		}
		resultsByTeam := make(map[string]schedule.HeatResult, len(results))
		for _, res := range results {
			resultsByTeam[res.TeamID] = res
		}

		divisions, err := s.schedule.ListDivisionsForRound(rd.ID)
		if err != nil {
			http.Error(w, "could not list divisions", http.StatusInternalServerError)
			return
		}

		var standings []standingsEntryResponse
		if len(divisions) > 0 {
			for _, d := range divisions {
				name := d.Name
				standings = append(standings, buildStandingsEntry(nil, &d.ID, &name, d.TeamIDs, resultsByTeam))
			}
		} else {
			groups, err := s.rounds.ListGroups(rd.ID)
			if err != nil {
				http.Error(w, "could not list groups", http.StatusInternalServerError)
				return
			}
			for _, g := range groups {
				standings = append(standings, buildStandingsEntry(&g.ID, nil, nil, g.TeamIDs, resultsByTeam))
			}
		}
		if standings == nil {
			standings = []standingsEntryResponse{}
		}

		resp = append(resp, standingsRoundResponse{
			ID:          rd.ID,
			RoundNumber: rd.RoundNumber,
			Status:      string(rd.Status),
			Standings:   standings,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"rounds": resp})
}

func buildStandingsEntry(groupID, divisionID *int64, divisionName *string, teamIDs []string, resultsByTeam map[string]schedule.HeatResult) standingsEntryResponse {
	var teamResults []ranking.TeamResult
	for _, teamID := range teamIDs {
		res, ok := resultsByTeam[teamID]
		if !ok {
			continue
		}
		teamResults = append(teamResults, ranking.TeamResult{
			TeamID:      teamID,
			TimeSeconds: res.TimeSeconds,
			Status:      ranking.Status(res.Status),
		})
	}
	ranked := ranking.Rank(teamResults)
	rankedTeams := make([]rankedTeamResponse, len(ranked))
	for i, res := range ranked {
		rankedTeams[i] = rankedTeamResponse{
			Rank:        i + 1,
			TeamID:      res.TeamID,
			TimeSeconds: res.TimeSeconds,
			Status:      string(res.Status),
		}
	}
	return standingsEntryResponse{
		GroupID:      groupID,
		DivisionID:   divisionID,
		DivisionName: divisionName,
		RankedTeams:  rankedTeams,
	}
}
