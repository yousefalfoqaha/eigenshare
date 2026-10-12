package user

import (
	"fmt"
	"regexp"

	"eigenshare/internal/security"
	"eigenshare/internal/timestamp"
)

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)

type User struct {
	ID           string              `json:"id"`
	Username     string              `json:"username"`
	PasswordHash string              `json:"-"`
	IsAdmin      bool                `json:"isAdmin"`
	CreatedAt    timestamp.Timestamp `json:"createdAt"`
	Version      int64               `json:"-"`
}

func validateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username is required: %w", ErrInvalidUsername)
	}
	if !validUsername.MatchString(username) {
		return fmt.Errorf("username may only contain letters, numbers, hyphens and underscores (3-64 chars): %w", ErrInvalidUsername)
	}
	return nil
}

func NewUser(id, username, passwordHash string, isAdmin bool) (*User, error) {
	if err := validateUsername(username); err != nil {
		return nil, err
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
		Version:      1,
	}, nil
}
