package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/schedule"
)

type updateHeatRequest struct {
	PlannedStart string `json:"planned_start"`
}

func (s *Server) handleUpdateHeat(w http.ResponseWriter, r *http.Request) {
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

	var req updateHeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startAt, err := parseStartAt(&req.PlannedStart)
	if err != nil {
		http.Error(w, "planned_start must be RFC3339", http.StatusBadRequest)
		return
	}

	updated, err := s.schedule.UpdateHeatPlannedStart(heatID, *startAt)
	if err != nil {
		http.Error(w, "could not update heat", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(heatToResponse(*updated))
}
