package auth

import (
	"net/http"

	"eigenshare/internal/apperr"
)

var (
	ErrInvalidCredentials = apperr.New(http.StatusUnauthorized, "invalid credentials")
	ErrSessionNotFound    = apperr.New(http.StatusUnauthorized, "session not found")
)
