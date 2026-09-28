package user

import "errors"

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidUsername = errors.New("invalid username")

	ErrUsernameTaken = errors.New("username already taken")
	ErrUserNotFound  = errors.New("user not found")

	ErrCannotDeleteSelf      = errors.New("cannot delete your own account")
	ErrCannotDeleteLastAdmin = errors.New("cannot delete the last admin")
	ErrCannotDemoteSelf      = errors.New("cannot remove admin from your own account")
)
