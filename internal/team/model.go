package team

type Team struct {
	ID           int64
	TournamentID int64
	Name         string
	Club         string
	ExtraFields  map[string]string
}
