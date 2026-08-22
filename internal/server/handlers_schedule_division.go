package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/schedule"
)

type divisionScheduleAssignment struct {
	DivisionID int64 `json:"division_id"`
	CourseID   int64 `json:"course_id"`
}

type scheduleDivisionsRequest struct {
	Assignments []divisionScheduleAssignment `json:"assignments"`
	StartAt     *string                      `json:"start_at"`
}

func (s *Server) handleScheduleDivisions(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req scheduleDivisionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startAt, err := parseStartAt(req.StartAt)
	if err != nil {
		http.Error(w, "start_at must be RFC3339", http.StatusBadRequest)
		return
	}
	if len(req.Assignments) == 0 {
		http.Error(w, "at least one assignment is required", http.StatusBadRequest)
		return
	}

	assignments := make([]schedule.DivisionAssignment, len(req.Assignments))
	for i, a := range req.Assignments {
		assignments[i] = schedule.DivisionAssignment{DivisionID: a.DivisionID, CourseID: a.CourseID}
	}

	heats, err := s.schedule.ScheduleDivisionHeats(tournamentID, assignments, startAt)
	if err != nil {
		if err == schedule.ErrCourseNotFound {
			http.Error(w, "unknown course", http.StatusBadRequest)
			return
		}
		if err == schedule.ErrDivisionNotFound {
			http.Error(w, "unknown division", http.StatusBadRequest)
			return
		}
		if err == schedule.ErrDivisionAlreadyScheduled {
			http.Error(w, "one or more divisions already have a scheduled heat", http.StatusConflict)
			return
		}
		http.Error(w, "could not schedule divisions", http.StatusInternalServerError)
		return
	}

	resp := make([]heatResponse, len(heats))
	for i, h := range heats {
		resp[i] = heatToResponse(h)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"heats": resp})
}
