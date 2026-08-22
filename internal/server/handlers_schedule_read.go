package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tournamentstudio/internal/schedule"
)

// scheduleHeatResponse's Results field reuses schedule.HeatResult
// directly (it already carries json tags -- see Task 4) rather than a
// separate response type. Each entry's own "heat_id" is technically
// redundant with the enclosing heat's "id", but not wrong, and it isn't
// worth a third near-identical struct just to drop one field.
type scheduleHeatResponse struct {
	ID             int64                 `json:"id"`
	RoundID        int64                 `json:"round_id"`
	GroupID        *int64                `json:"group_id"`
	DivisionID     *int64                `json:"division_id"`
	CourseID       int64                 `json:"course_id"`
	PlannedStart   string                `json:"planned_start"`
	EffectiveStart string                `json:"effective_start"`
	Status         string                `json:"status"`
	Results        []schedule.HeatResult `json:"results"`
}

func (s *Server) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	heats, err := s.schedule.ListHeatsForTournament(tournamentID)
	if err != nil {
		http.Error(w, "could not list heats", http.StatusInternalServerError)
		return
	}

	courseCache := make(map[int64]*schedule.Course, len(heats))
	resp := make([]scheduleHeatResponse, 0, len(heats))
	for _, h := range heats {
		course, ok := courseCache[h.CourseID]
		if !ok {
			course, err = s.schedule.GetCourse(h.CourseID)
			if err != nil {
				http.Error(w, "could not get course", http.StatusInternalServerError)
				return
			}
			courseCache[h.CourseID] = course
		}
		effective := h.PlannedStart.Add(time.Duration(course.DelayOffsetSeconds) * time.Second)

		results, err := s.schedule.ListHeatResults(h.ID)
		if err != nil {
			http.Error(w, "could not list heat results", http.StatusInternalServerError)
			return
		}

		resp = append(resp, scheduleHeatResponse{
			ID:             h.ID,
			RoundID:        h.RoundID,
			GroupID:        h.GroupID,
			DivisionID:     h.DivisionID,
			CourseID:       h.CourseID,
			PlannedStart:   h.PlannedStart.Format(time.RFC3339),
			EffectiveStart: effective.Format(time.RFC3339),
			Status:         string(h.Status),
			Results:        results,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"heats": resp})
}
