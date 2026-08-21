package tournament

type Tournament struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	SportPluginID    string `json:"sport_plugin_id"`
	TournamentTypeID string `json:"tournament_type_id"`
	Language         string `json:"language"`
	Status           string `json:"status"`
}
