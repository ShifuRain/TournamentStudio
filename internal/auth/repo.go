package auth

import (
	"database/sql"
	"errors"
	"fmt"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"tournamentstudio/internal/store"
)

var ErrNotFound = errors.New("user not found")
var ErrDuplicateUsername = errors.New("username already exists")
var ErrInvalidRole = errors.New("invalid role")

type Repo struct {
	db *sql.DB
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{db: s.DB}
}

func isUniqueConstraintErr(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
	}
	return false
}

func isValidRole(role Role) bool {
	switch role {
	case RoleOrganizer, RoleTimeEntry, RoleSpectator:
		return true
	default:
		return false
	}
}

func (r *Repo) Create(username, plainPassword string, role Role) (*User, error) {
	if !isValidRole(role) {
		return nil, ErrInvalidRole
	}

	hash, err := HashPassword(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	res, err := r.db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, string(role))
	if err != nil {
		if isUniqueConstraintErr(err) {
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

func (r *Repo) FindByID(id int64) (*User, error) {
	row := r.db.QueryRow(`SELECT id, username, password_hash, role FROM users WHERE id = ?`, id)
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

func (r *Repo) Count() (int, error) {
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
