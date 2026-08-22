package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tournamentstudio/internal/schedule"
)

func courseToResponse(c *schedule.Course) schedule.Course {
	return *c
}

type createCourseRequest struct {
	Name                string `json:"name"`
	HeatIntervalSeconds int    `json:"heat_interval_seconds"`
}

func (s *Server) handleCreateCourse(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	var req createCourseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.HeatIntervalSeconds < 1 {
		http.Error(w, "heat_interval_seconds must be at least 1", http.StatusBadRequest)
		return
	}

	c, err := s.schedule.CreateCourse(tournamentID, req.Name, req.HeatIntervalSeconds)
	if err != nil {
		http.Error(w, "could not create course", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(courseToResponse(c))
}

func (s *Server) handleListCourses(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}

	courses, err := s.schedule.ListCourses(tournamentID)
	if err != nil {
		http.Error(w, "could not list courses", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"courses": courses})
}

type updateCourseRequest struct {
	Name                *string `json:"name"`
	HeatIntervalSeconds *int    `json:"heat_interval_seconds"`
	DelayOffsetSeconds  *int    `json:"delay_offset_seconds"`
}

func (s *Server) handleUpdateCourse(w http.ResponseWriter, r *http.Request) {
	tournamentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tournament id", http.StatusBadRequest)
		return
	}
	courseID, err := strconv.ParseInt(r.PathValue("course_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid course id", http.StatusBadRequest)
		return
	}

	existing, err := s.schedule.GetCourse(courseID)
	if err != nil {
		if err == schedule.ErrCourseNotFound {
			http.Error(w, "course not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not get course", http.StatusInternalServerError)
		return
	}
	if existing.TournamentID != tournamentID {
		http.Error(w, "course not found", http.StatusNotFound)
		return
	}

	var req updateCourseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name != nil && *req.Name == "" {
		http.Error(w, "name cannot be empty", http.StatusBadRequest)
		return
	}
	if req.HeatIntervalSeconds != nil && *req.HeatIntervalSeconds < 1 {
		http.Error(w, "heat_interval_seconds must be at least 1", http.StatusBadRequest)
		return
	}

	updated, err := s.schedule.UpdateCourse(courseID, schedule.CourseUpdate{
		Name:                req.Name,
		HeatIntervalSeconds: req.HeatIntervalSeconds,
		DelayOffsetSeconds:  req.DelayOffsetSeconds,
	})
	if err != nil {
		http.Error(w, "could not update course", http.StatusInternalServerError)
		return
	}

	if req.DelayOffsetSeconds != nil {
		msg, _ := json.Marshal(map[string]any{
			"type":                 "delay_offset_changed",
			"course_id":            updated.ID,
			"delay_offset_seconds": updated.DelayOffsetSeconds,
		})
		s.hub.broadcast(tournamentID, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(courseToResponse(updated))
}
