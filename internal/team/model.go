package team

type Team struct {
	ID           int64             `json:"id"`
	TournamentID int64             `json:"tournament_id"`
	Name         string            `json:"name"`
	Club         string            `json:"club"`
	ExtraFields  map[string]string `json:"extra_fields"`
}
