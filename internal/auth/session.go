package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"tournamentstudio/internal/store"
)

var ErrInvalidSession = errors.New("invalid session")

type Session struct {
	Token  string
	UserID int64
	Role   Role
}

type SessionRepo struct {
	db *sql.DB
}

func NewSessionRepo(s *store.Store) *SessionRepo {
	return &SessionRepo{db: s.DB}
}

func (r *SessionRepo) Create(userID int64, role Role) (*Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	if _, err := r.db.Exec(`INSERT INTO sessions (token, user_id, role) VALUES (?, ?, ?)`, token, userID, string(role)); err != nil {
		return nil, err
	}
	return &Session{Token: token, UserID: userID, Role: role}, nil
}

func (r *SessionRepo) Delete(token string) error {
	_, err := r.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (r *SessionRepo) Find(token string) (*Session, error) {
	row := r.db.QueryRow(`SELECT token, user_id, role FROM sessions WHERE token = ?`, token)
	var s Session
	var role string
	if err := row.Scan(&s.Token, &s.UserID, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	s.Role = Role(role)
	return &s, nil
}
