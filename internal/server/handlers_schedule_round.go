package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tournamentstudio/internal/round"
	"tournamentstudio/internal/schedule"
)

type scheduleAssignment struct {
	GroupID  int64 `json:"group_id"`
	CourseID int64 `json:"course_id"`
}

type scheduleRoundRequest struct {
	Assignments []scheduleAssignment `json:"assignments"`
	StartAt     *string              `json:"start_at"`
}

type heatResponse struct {
	ID           int64  `json:"id"`
	RoundID      int64  `json:"round_id"`
	GroupID      *int64 `json:"group_id"`
	DivisionID   *int64 `json:"division_id"`
	CourseID     int64  `json:"course_id"`
	PlannedStart string `json:"planned_start"`
	Status       string `json:"status"`
}

func heatToResponse(h schedule.Heat) heatResponse {
	return heatResponse{
		ID:           h.ID,
		RoundID:      h.RoundID,
		GroupID:      h.GroupID,
		DivisionID:   h.DivisionID,
		CourseID:     h.CourseID,
		PlannedStart: h.PlannedStart.Format(time.RFC3339),
		Status:       string(h.Status),
	}
}

func parseStartAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Server) handleScheduleRound(w http.ResponseWriter, r *http.Request) {
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

	var req scheduleRoundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	startAt, err := parseStartAt(req.StartAt)
	if err != nil {
		http.Error(w, "start_at must be RFC3339", http.StatusBadRequest)
		return
	}

	groups, err := s.rounds.ListGroups(roundID)
	if err != nil {
		http.Error(w, "could not list groups", http.StatusInternalServerError)
		return
	}
	if len(req.Assignments) != len(groups) {
		http.Error(w, "assignments must cover every group of the round exactly once", http.StatusBadRequest)
		return
	}
	roundGroupIDs := make(map[int64]bool, len(groups))
	for _, g := range groups {
		roundGroupIDs[g.ID] = true
	}
	seen := make(map[int64]bool, len(req.Assignments))
	assignments := make([]schedule.GroupAssignment, 0, len(req.Assignments))
	for _, a := range req.Assignments {
		if !roundGroupIDs[a.GroupID] || seen[a.GroupID] {
			http.Error(w, "assignments must cover every group of the round exactly once", http.StatusBadRequest)
			return
		}
		seen[a.GroupID] = true
		assignments = append(assignments, schedule.GroupAssignment{GroupID: a.GroupID, CourseID: a.CourseID})
	}

	heats, err := s.schedule.ScheduleGroupHeats(tournamentID, roundID, assignments, startAt)
	if err != nil {
		if err == schedule.ErrCourseNotFound {
			http.Error(w, "unknown course", http.StatusBadRequest)
			return
		}
		if err == schedule.ErrGroupAlreadyScheduled {
			http.Error(w, "one or more groups already have a scheduled heat", http.StatusConflict)
			return
		}
		http.Error(w, "could not schedule round", http.StatusInternalServerError)
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
