package web

import "errors"

var (
	errInvalidRequest     = errors.New("invalid request")
	errInvalidCredentials = errors.New("invalid credentials")
	errForbidden          = errors.New("insufficient permission")
)
