package auth

import "golang.org/x/crypto/bcrypt"

type Role string

const (
	RoleOrganizer Role = "organizer"
	RoleTimeEntry Role = "time_entry"
	RoleSpectator Role = "spectator"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         Role
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
