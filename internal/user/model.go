package user

import (
	"fmt"
	"regexp"

	"eigenshare/internal/security"
	"eigenshare/internal/timestamp"
)

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type User struct {
	ID           string              `json:"id"`
	Username     string              `json:"username"`
	PasswordHash string              `json:"-"`
	IsAdmin      bool                `json:"isAdmin"`
	CreatedAt    timestamp.Timestamp `json:"createdAt"`
}

func NewUser(id, username, passwordHash string, isAdmin bool) (*User, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required: %w", ErrInvalidUsername)
	}
	if !validUsername.MatchString(username) {
		return nil, fmt.Errorf("username may only contain letters, numbers, hyphens and underscores (max 64 chars): %w", ErrInvalidUsername)
	}
	if passwordHash == "" {
		return nil, fmt.Errorf("password hash is required: %w", security.ErrInvalidPassword)
	}

	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		IsAdmin:      isAdmin,
		CreatedAt:    timestamp.Now(),
	}, nil
}
