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

var validResultStatus = map[string]bool{
	"DNF": true,
	"DSQ": true,
	"DNS": true,
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

	// Validate every entry before writing anything: a malformed entry
	// anywhere in the (randomly-ordered) map must reject the whole
	// request with nothing written, rather than partially committing
	// entries that happened to be visited first.
	results := make([]round.Result, 0, len(req))
	for teamID, entry := range req {
		if entry.TimeSeconds == nil && entry.Status == "" {
			http.Error(w, "each result must have either time_seconds or status", http.StatusBadRequest)
			return
		}
		if entry.Status != "" && !validResultStatus[entry.Status] {
			http.Error(w, "status must be one of DNF, DSQ, DNS", http.StatusBadRequest)
			return
		}
		results = append(results, round.Result{
			TeamID:      teamID,
			TimeSeconds: entry.TimeSeconds,
			Status:      entry.Status,
		})
	}

	if err := s.rounds.SubmitResults(roundID, results); err != nil {
		http.Error(w, "could not save results", http.StatusInternalServerError)
		return
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	validTeamIDs := make(map[string]bool)
	for _, g := range groups {
		for _, teamID := range g.TeamIDs {
			validTeamIDs[teamID] = true
		}
	}

	allResults, err := s.rounds.ListResults(roundID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}

	haveResult := make(map[string]bool, len(allResults))
	for _, res := range allResults {
		haveResult[res.TeamID] = true
	}

	allComplete := len(validTeamIDs) > 0
	for teamID := range validTeamIDs {
		if !haveResult[teamID] {
			allComplete = false
			break
		}
	}

	if allComplete {
		if err := s.rounds.SetStatus(roundID, round.StatusClosed); err != nil {
			http.Error(w, "could not close round", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"results_recorded": len(allResults)})
}
