package server

import (
	"net/http"
	"strconv"

	"tournamentstudio/internal/plugin"
	"tournamentstudio/internal/round"
	"tournamentstudio/internal/tournament"
)

func (s *Server) findTournamentType(id string) *plugin.TournamentTypePlugin {
	for _, ttp := range s.plugins.TournamentTypes() {
		if ttp.ID == id {
			return ttp
		}
	}
	return nil
}

// roundContext bundles everything handleNextRound and handleComputeDivisions
// both need after confirming a round is closed and ready to be acted on:
// the round and its parent tournament, the tournament's type plugin, the
// round's groups, and a lookup from team ID to that team's result.
type roundContext struct {
	tournamentID  int64
	round         *round.PrePhaseRound
	tournament    *tournament.Tournament
	ttp           *plugin.TournamentTypePlugin
	groups        []round.Group
	resultsByTeam map[string]round.Result
}

// loadClosedRoundContext parses the {id}/{round_id} path values, loads the
// round and confirms it belongs to the tournament and is closed, then loads
// the tournament, its type plugin, the round's groups, and its results
// indexed by team ID. On any failure it writes the appropriate error
// response itself and returns (nil, false); callers should return
// immediately in that case.
func (s *Server) loadClosedRoundContext(w http.ResponseWriter, r *http.Request) (*roundContext, bool) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return nil, false
	}
	roundID, err := strconv.ParseInt(r.PathValue("round_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid round id", http.StatusBadRequest)
		return nil, false
	}

	pr, err := s.rounds.GetRound(roundID)
	if err != nil {
		if err == round.ErrNotFound {
			http.Error(w, "round not found", http.StatusNotFound)
			return nil, false
		}
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return nil, false
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "round not found", http.StatusNotFound)
		return nil, false
	}
	if pr.Status != round.StatusClosed {
		http.Error(w, "round must be closed first", http.StatusConflict)
		return nil, false
	}

	tour, err := s.tournaments.Get(tournamentID)
	if err != nil {
		http.Error(w, "could not get tournament", http.StatusInternalServerError)
		return nil, false
	}
	ttp := s.findTournamentType(tour.TournamentTypeID)
	if ttp == nil {
		http.Error(w, "tournament type plugin not found", http.StatusInternalServerError)
		return nil, false
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return nil, false
	}
	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return nil, false
	}
	resultsByTeam := make(map[string]round.Result, len(results))
	for _, res := range results {
		resultsByTeam[res.TeamID] = res
	}

	return &roundContext{
		tournamentID:  tournamentID,
		round:         pr,
		tournament:    tour,
		ttp:           ttp,
		groups:        groups,
		resultsByTeam: resultsByTeam,
	}, true
}
