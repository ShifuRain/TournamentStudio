package schedule

import (
	"database/sql"

	"tournamentstudio/internal/store"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}
