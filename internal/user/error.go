package user

import (
	"net/http"

	"eigenshare/internal/apperr"
)

var (
	ErrUsernameTaken         = apperr.New(http.StatusConflict, "username already taken")
	ErrUserNotFound          = apperr.New(http.StatusNotFound, "user not found")
	ErrCannotDeleteSelf      = apperr.New(http.StatusConflict, "cannot delete your own account")
	ErrCannotDeleteLastAdmin = apperr.New(http.StatusConflict, "cannot delete the last admin")
	ErrCannotDemoteSelf      = apperr.New(http.StatusConflict, "cannot remove admin from your own account")
)
