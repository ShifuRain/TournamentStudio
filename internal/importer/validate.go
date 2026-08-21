package importer

import (
	"strings"

	"tournamentstudio/internal/team"
)

type RowProblem struct {
	RowIndex int
	Message  string
}

type ValidationResult struct {
	Teams    []team.Team
	Problems []RowProblem
}

func Validate(tournamentID int64, rows []map[string]string) ValidationResult {
	var result ValidationResult

	for i, row := range rows {
		name := strings.TrimSpace(row["name"])
		if name == "" {
			result.Problems = append(result.Problems, RowProblem{RowIndex: i, Message: "missing team name"})
			continue
		}

		extra := make(map[string]string)
		for k, v := range row {
			if k == "name" || k == "club" {
				continue
			}
			extra[k] = v
		}

		result.Teams = append(result.Teams, team.Team{
			TournamentID: tournamentID,
			Name:         name,
			Club:         strings.TrimSpace(row["club"]),
			ExtraFields:  extra,
		})
	}

	return result
}
