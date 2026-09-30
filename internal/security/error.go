package security

import "errors"

var (
	// ErrInvalidPassword is a password policy violation (too short, too long, ...).
	ErrInvalidPassword = errors.New("invalid password")
	// ErrWrongPassword is returned when a submitted password doesn't match.
	ErrWrongPassword = errors.New("incorrect password")
	// ErrPasswordTooLong is the bcrypt 72-byte limit.
	ErrPasswordTooLong = errors.New("password exceeds 72 bytes")
)
