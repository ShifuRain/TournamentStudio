package team

import (
	"database/sql"
	"encoding/json"

	"tournamentstudio/internal/store"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) Create(t Team) (*Team, error) {
	if t.ExtraFields == nil {
		t.ExtraFields = map[string]string{}
	}
	extra, err := json.Marshal(t.ExtraFields)
	if err != nil {
		return nil, err
	}

	res, err := r.db.Exec(
		`INSERT INTO teams (tournament_id, name, club, extra_fields) VALUES (?, ?, ?, ?)`,
		t.TournamentID, t.Name, t.Club, string(extra),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	t.ID = id
	return &t, nil
}

func (r *Repo) ListByTournament(tournamentID int64) ([]Team, error) {
	rows, err := r.db.Query(`SELECT id, tournament_id, name, club, extra_fields FROM teams WHERE tournament_id = ? ORDER BY id`, tournamentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Team{}
	for rows.Next() {
		var t Team
		var extra string
		if err := rows.Scan(&t.ID, &t.TournamentID, &t.Name, &t.Club, &extra); err != nil {
			return nil, err
		}
		if extra != "" {
			if err := json.Unmarshal([]byte(extra), &t.ExtraFields); err != nil {
				return nil, err
			}
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
