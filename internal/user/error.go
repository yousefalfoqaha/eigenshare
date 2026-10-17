package user

import "errors"

var (
	ErrInvalidUsername       = errors.New("invalid username")
	ErrUsernameTaken         = errors.New("username already taken")
	ErrUserNotFound          = errors.New("user not found")
	ErrVersionConflict       = errors.New("user was modified by another request")
	ErrCannotDeleteSelf      = errors.New("cannot delete your own account")
	ErrCannotDeleteLastAdmin = errors.New("cannot delete the last admin")
	ErrCannotDemoteSelf      = errors.New("cannot remove admin from your own account")
)
