package user

import (
	"net/http"
	"regexp"

	"eigenshare/internal/apperr"
	"eigenshare/internal/timestamp"

	"golang.org/x/crypto/bcrypt"
)

var (
	validUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	hashCost      = 12
)

type User struct {
	ID           string              `json:"id"`
	Username     string              `json:"username"`
	PasswordHash string              `json:"-"`
	IsAdmin      bool                `json:"isAdmin"`
	CreatedAt    timestamp.Timestamp `json:"createdAt"`
}

func NewUser(id, username, passwordHash string, isAdmin bool) (*User, error) {
	if username == "" {
		return nil, apperr.New(http.StatusBadRequest, "username is required")
	}
	if !validUsername.MatchString(username) {
		return nil, apperr.New(http.StatusBadRequest, "username may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}
	if passwordHash == "" {
		return nil, apperr.New(http.StatusBadRequest, "password hash is required")
	}

	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		IsAdmin:      isAdmin,
		CreatedAt:    timestamp.Now(),
	}, nil
}

func (u *User) VerifyPassword(password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
