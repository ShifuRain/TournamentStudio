package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("user not found")
var ErrDuplicateUsername = errors.New("username already exists")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func (r *Repo) Create(username, plainPassword string, role Role) (*User, error) {
	hash, err := HashPassword(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	res, err := r.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, string(role))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ErrDuplicateUsername
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Username: username, PasswordHash: hash, Role: role}, nil
}

func (r *Repo) FindByUsername(username string) (*User, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, role FROM users WHERE username = ?`, username)
	var u User
	var role string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Role = Role(role)
	return &u, nil
}
