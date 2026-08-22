package schedule

import (
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrDivisionNotFound = errors.New("division not found")

type NewDivision struct {
	Name    string
	TeamIDs []string
}

// CreateDivisions persists every division in one transaction, rolling
// back on any error.
func (r *Repo) CreateDivisions(tournamentID, roundID int64, divisions []NewDivision) ([]Division, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	created := make([]Division, 0, len(divisions))
	for _, d := range divisions {
		teamIDsJSON, err := json.Marshal(d.TeamIDs)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		res, err := tx.Exec(
			`INSERT INTO divisions (tournament_id, round_id, name, team_ids) VALUES (?, ?, ?, ?)`,
			tournamentID, roundID, d.Name, string(teamIDsJSON),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Division{ID: id, TournamentID: tournamentID, RoundID: roundID, Name: d.Name, TeamIDs: d.TeamIDs})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *Repo) ListDivisionsForRound(roundID int64) ([]Division, error) {
	rows, err := r.db.Query(
		`SELECT id, tournament_id, round_id, name, team_ids FROM divisions WHERE round_id = ? ORDER BY id`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var divisions []Division
	for rows.Next() {
		var d Division
		var teamIDsJSON string
		if err := rows.Scan(&d.ID, &d.TournamentID, &d.RoundID, &d.Name, &teamIDsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(teamIDsJSON), &d.TeamIDs); err != nil {
			return nil, err
		}
		divisions = append(divisions, d)
	}
	return divisions, rows.Err()
}

func (r *Repo) GetDivision(id int64) (*Division, error) {
	row := r.db.QueryRow(`SELECT id, tournament_id, round_id, name, team_ids FROM divisions WHERE id = ?`, id)
	var d Division
	var teamIDsJSON string
	if err := row.Scan(&d.ID, &d.TournamentID, &d.RoundID, &d.Name, &teamIDsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDivisionNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(teamIDsJSON), &d.TeamIDs); err != nil {
		return nil, err
	}
	return &d, nil
}
