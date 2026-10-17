package web

import "errors"

var (
	ErrInvalidRequest     = errors.New("invalid request")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrForbidden          = errors.New("insufficient permission")
)
