package session

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")

	ErrInvalidToken  = errors.New("invalid session token")
	ErrInvalidUserID = errors.New("invalid user id")
	ErrInvalidExpiry = errors.New("invalid expiry")
)
