package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
)

type submitResultsRequest map[string]struct {
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}

func (s *Server) handleSubmitResults(w http.ResponseWriter, r *http.Request) {
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

	var req submitResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	for teamID, entry := range req {
		if entry.TimeSeconds == nil && entry.Status == "" {
			http.Error(w, "each result must have either time_seconds or status", http.StatusBadRequest)
			return
		}
		if err := s.rounds.SubmitResult(roundID, round.Result{
			TeamID:      teamID,
			TimeSeconds: entry.TimeSeconds,
			Status:      entry.Status,
		}); err != nil {
			http.Error(w, "could not save result", http.StatusInternalServerError)
			return
		}
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	expected := 0
	for _, g := range groups {
		expected += len(g.TeamIDs)
	}

	results, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}

	if expected > 0 && len(results) >= expected {
		if err := s.rounds.SetStatus(roundID, round.StatusClosed); err != nil {
			http.Error(w, "could not close round", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"results_recorded": len(results)})
}
