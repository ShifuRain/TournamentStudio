package tournament

import (
	"database/sql"
	"errors"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("tournament not found")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) Create(t Tournament) (*Tournament, error) {
	res, err := r.db.Exec(
		`INSERT INTO tournaments (name, sport_plugin_id, tournament_type_id, language, status) VALUES (?, ?, ?, ?, ?)`,
		t.Name, t.SportPluginID, t.TournamentTypeID, t.Language, "draft",
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	t.ID = id
	t.Status = "draft"
	return &t, nil
}

func (r *Repo) Get(id int64) (*Tournament, error) {
	row := r.db.QueryRow(`SELECT id, name, sport_plugin_id, tournament_type_id, language, status FROM tournaments WHERE id = ?`, id)
	var t Tournament
	if err := row.Scan(&t.ID, &t.Name, &t.SportPluginID, &t.TournamentTypeID, &t.Language, &t.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repo) List() ([]Tournament, error) {
	rows, err := r.db.Query(`SELECT id, name, sport_plugin_id, tournament_type_id, language, status FROM tournaments ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Tournament{}
	for rows.Next() {
		var t Tournament
		if err := rows.Scan(&t.ID, &t.Name, &t.SportPluginID, &t.TournamentTypeID, &t.Language, &t.Status); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
