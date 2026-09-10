package auth

import (
	"fmt"
	"time"

	"eigenshare/internal/timestamp"
)

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
