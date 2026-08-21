package round

import (
	"database/sql"
	"encoding/json"
	"errors"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("round not found")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) CreateRound(tournamentID int64, roundNumber int, groups [][]string) (*PrePhaseRound, []Group, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, err
	}

	res, err := tx.Exec(
		`INSERT INTO pre_phase_rounds (tournament_id, round_number, status) VALUES (?, ?, ?)`,
		tournamentID, roundNumber, string(StatusOpen),
	)
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	roundID, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}

	createdGroups := make([]Group, 0, len(groups))
	for _, teamIDs := range groups {
		teamIDsJSON, err := json.Marshal(teamIDs)
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		gres, err := tx.Exec(`INSERT INTO groups (round_id, team_ids) VALUES (?, ?)`, roundID, string(teamIDsJSON))
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		groupID, err := gres.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		createdGroups = append(createdGroups, Group{ID: groupID, RoundID: roundID, TeamIDs: teamIDs})
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &PrePhaseRound{ID: roundID, TournamentID: tournamentID, RoundNumber: roundNumber, Status: StatusOpen}, createdGroups, nil
}

func (r *Repo) GetRound(id int64) (*PrePhaseRound, error) {
	row := r.db.QueryRow(`SELECT id, tournament_id, round_number, status FROM pre_phase_rounds WHERE id = ?`, id)
	var pr PrePhaseRound
	var status string
	if err := row.Scan(&pr.ID, &pr.TournamentID, &pr.RoundNumber, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	pr.Status = Status(status)
	return &pr, nil
}

func (r *Repo) ListGroups(roundID int64) ([]Group, error) {
	rows, err := r.db.Query(`SELECT id, round_id, team_ids FROM groups WHERE round_id = ? ORDER BY id`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		var teamIDsJSON string
		if err := rows.Scan(&g.ID, &g.RoundID, &teamIDsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(teamIDsJSON), &g.TeamIDs); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r *Repo) SetStatus(roundID int64, status Status) error {
	_, err := r.db.Exec(`UPDATE pre_phase_rounds SET status = ? WHERE id = ?`, string(status), roundID)
	return err
}

func (r *Repo) SubmitResult(roundID int64, res Result) error {
	_, err := r.db.Exec(
		`INSERT INTO round_results (round_id, team_id, time_seconds, status) VALUES (?, ?, ?, ?)
		 ON CONFLICT(round_id, team_id) DO UPDATE SET time_seconds = excluded.time_seconds, status = excluded.status`,
		roundID, res.TeamID, res.TimeSeconds, nullIfEmpty(res.Status),
	)
	return err
}

func (r *Repo) ListResults(roundID int64) ([]Result, error) {
	rows, err := r.db.Query(`SELECT team_id, time_seconds, status FROM round_results WHERE round_id = ?`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var res Result
		var timeSeconds sql.NullFloat64
		var status sql.NullString
		if err := rows.Scan(&res.TeamID, &timeSeconds, &status); err != nil {
			return nil, err
		}
		if timeSeconds.Valid {
			v := timeSeconds.Float64
			res.TimeSeconds = &v
		}
		if status.Valid {
			res.Status = status.String
		}
		results = append(results, res)
	}
	return results, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
