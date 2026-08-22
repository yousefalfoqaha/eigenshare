package user

import (
	"eigenshare/backend/timestamp"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	validUsername = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

	ErrUsernameTaken         = errors.New("username already taken")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrSessionNotFound       = errors.New("session not found")
	ErrUserNotFound          = errors.New("user not found")
	ErrCannotDeleteSelf      = errors.New("cannot delete your own account")
	ErrCannotDeleteLastAdmin = errors.New("cannot delete the last admin")
	ErrCannotDemoteSelf      = errors.New("cannot remove admin from your own account")
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
		return nil, fmt.Errorf("username is required")
	}
	if !validUsername.MatchString(username) {
		return nil, fmt.Errorf("username may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}
	if passwordHash == "" {
		return nil, fmt.Errorf("password hash is required")
	}

	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		IsAdmin:      isAdmin,
		CreatedAt:    timestamp.Now(),
	}, nil
}

type CreateRequest struct {
	Username string `form:"username" json:"username"`
	Password string `form:"password" json:"password"`
	IsAdmin  bool   `form:"isAdmin" json:"isAdmin"`
}

type UpdateRequest struct {
	Username string `form:"username" json:"username"`
	Password string `form:"password" json:"password"`
	IsAdmin  bool   `form:"isAdmin" json:"isAdmin"`
}

type Session struct {
	Token     string               `json:"token"`
	UserID    string               `json:"userId"`
	CreatedAt timestamp.Timestamp  `json:"createdAt"`
	ExpiresAt *timestamp.Timestamp `json:"expiresAt"`
}

func NewSession(token, userID string, expiryMinutes int) (*Session, error) {
	if token == "" {
		return nil, fmt.Errorf("session token is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id is required")
	}
	if expiryMinutes < 0 {
		return nil, fmt.Errorf("expiry minutes must be non-negative")
	}

	createdAt := timestamp.Now()
	s := &Session{
		Token:     token,
		UserID:    userID,
		CreatedAt: createdAt,
	}

	if expiryMinutes > 0 {
		expiresAt := timestamp.Timestamp(time.Time(createdAt).Add(time.Duration(expiryMinutes) * time.Minute))
		s.ExpiresAt = &expiresAt
	}

	return s, nil
}
