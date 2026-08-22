package schedule

type Course struct {
	ID                  int64  `json:"id"`
	TournamentID        int64  `json:"tournament_id"`
	Name                string `json:"name"`
	HeatIntervalSeconds int    `json:"heat_interval_seconds"`
	DelayOffsetSeconds  int    `json:"delay_offset_seconds"`
}
