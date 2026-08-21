package round

type Status string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

type PrePhaseRound struct {
	ID           int64
	TournamentID int64
	RoundNumber  int
	Status       Status
}

type Group struct {
	ID      int64
	RoundID int64
	TeamIDs []string
}
