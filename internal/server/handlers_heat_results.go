package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
)

type submitHeatResultsRequest map[string]struct {
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}

var validHeatResultStatus = map[string]bool{
	"DNF": true,
	"DSQ": true,
	"DNS": true,
}

func (s *Server) handleSubmitHeatResults(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	heatID, err := strconv.ParseInt(r.PathValue("heat_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid heat id", http.StatusBadRequest)
		return
	}

	heat, err := s.schedule.GetHeat(heatID)
	if err != nil {
		if err == schedule.ErrHeatNotFound {
			http.Error(w, "heat not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get heat", http.StatusInternalServerError)
		return
	}

	pr, err := s.rounds.GetRound(heat.RoundID)
	if err != nil {
		http.Error(w, "could not get round", http.StatusInternalServerError)
		return
	}
	if pr.TournamentID != tournamentID {
		http.Error(w, "heat not found", http.StatusNotFound)
		return
	}

	var validTeamIDsSource []string
	if heat.GroupID != nil {
		group, err := s.rounds.GetGroup(*heat.GroupID)
		if err != nil {
			http.Error(w, "could not get group", http.StatusInternalServerError)
			return
		}
		validTeamIDsSource = group.TeamIDs
	} else {
		division, err := s.schedule.GetDivision(*heat.DivisionID)
		if err != nil {
			http.Error(w, "could not get division", http.StatusInternalServerError)
			return
		}
		validTeamIDsSource = division.TeamIDs
	}

	var req submitHeatResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	results := make([]schedule.HeatResult, 0, len(req))
	for teamID, entry := range req {
		if entry.TimeSeconds == nil && entry.Status == "" {
			http.Error(w, "each result must have either time_seconds or status", http.StatusBadRequest)
			return
		}
		if entry.Status != "" && !validHeatResultStatus[entry.Status] {
			http.Error(w, "status must be one of DNF, DSQ, DNS", http.StatusBadRequest)
			return
		}
		results = append(results, schedule.HeatResult{
			TeamID:      teamID,
			TimeSeconds: entry.TimeSeconds,
			Status:      entry.Status,
		})
	}

	if err := s.schedule.SubmitHeatResults(heatID, results); err != nil {
		http.Error(w, "could not save results", http.StatusInternalServerError)
		return
	}

	validTeamIDs := make(map[string]bool, len(validTeamIDsSource))
	for _, id := range validTeamIDsSource {
		validTeamIDs[id] = true
	}

	allResults, err := s.schedule.ListHeatResults(heatID)
	if err != nil {
		http.Error(w, "could not list results", http.StatusInternalServerError)
		return
	}
	haveResult := make(map[string]bool, len(allResults))
	for _, res := range allResults {
		haveResult[res.TeamID] = true
	}

	heatComplete := len(validTeamIDs) > 0
	for teamID := range validTeamIDs {
		if !haveResult[teamID] {
			heatComplete = false
			break
		}
	}

	if heatComplete {
		if err := s.schedule.SetHeatStatus(heatID, schedule.HeatClosed); err != nil {
			http.Error(w, "could not close heat", http.StatusInternalServerError)
			return
		}

		if heat.GroupID != nil {
			// Only a round's own group-heats gate that round's closure --
			// its divisions' heats (added after the round is already
			// closed, once /divisions has run) must never reopen or
			// re-trigger this check.
			roundHeats, err := s.schedule.ListHeatsForRound(heat.RoundID)
			if err != nil {
				http.Error(w, "could not list round heats", http.StatusInternalServerError)
				return
			}
			roundComplete := true
			for _, h := range roundHeats {
				if h.GroupID != nil && h.Status != schedule.HeatClosed {
					roundComplete = false
					break
				}
			}
			if roundComplete {
				if err := s.rounds.SetStatus(heat.RoundID, round.StatusClosed); err != nil {
					http.Error(w, "could not close round", http.StatusInternalServerError)
					return
				}
			}
		}
	}

	msg, _ := json.Marshal(map[string]any{
		"type":    "result_submitted",
		"heat_id": heatID,
		"results": allResults,
	})
	s.hub.broadcast(tournamentID, msg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"results_recorded": len(allResults)})
}
