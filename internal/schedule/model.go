package schedule

import "time"

type Course struct {
	ID                  int64  `json:"id"`
	TournamentID        int64  `json:"tournament_id"`
	Name                string `json:"name"`
	HeatIntervalSeconds int    `json:"heat_interval_seconds"`
	DelayOffsetSeconds  int    `json:"delay_offset_seconds"`
}

type HeatStatus string

const (
	HeatScheduled HeatStatus = "scheduled"
	HeatClosed    HeatStatus = "closed"
)

type Heat struct {
	ID           int64
	RoundID      int64
	GroupID      *int64
	DivisionID   *int64
	CourseID     int64
	PlannedStart time.Time
	Status       HeatStatus
}

type HeatResult struct {
	HeatID      int64    `json:"heat_id"`
	TeamID      string   `json:"team_id"`
	TimeSeconds *float64 `json:"time_seconds"`
	Status      string   `json:"status"`
}
