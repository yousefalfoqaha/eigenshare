package security

import "errors"

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrWrongPassword   = errors.New("incorrect password")
	ErrPasswordTooLong = errors.New("password exceeds 72 bytes")
)
