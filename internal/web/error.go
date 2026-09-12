package web

import (
	"errors"
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type HTTPError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (e HTTPError) Error() string {
	return e.Message
}

func NewHTTPError(status int, message string) HTTPError {
	return HTTPError{Status: status, Message: message}
}

var errorRegistry = map[error]int{
	// 400 Bad Request
	share.ErrInvalidRequest:     http.StatusBadRequest,
	share.ErrInvalidChunkOffset: http.StatusBadRequest,

	// 401 Unauthorized
	auth.ErrInvalidCredentials: http.StatusUnauthorized,
	auth.ErrSessionNotFound:    http.StatusUnauthorized,

	// 404 Not Found
	share.ErrShareNotFound: http.StatusNotFound,
	share.ErrFileNotFound:  http.StatusNotFound,
	user.ErrUserNotFound:   http.StatusNotFound,

	// 409 Conflict
	share.ErrHandleExists:           http.StatusConflict,
	share.ErrShareVersionConflict:   http.StatusConflict,
	share.ErrShareNotComplete:       http.StatusConflict,
	share.ErrChunkOffsetExceedsSize: http.StatusConflict,
	user.ErrUsernameTaken:           http.StatusConflict,
	user.ErrCannotDeleteSelf:        http.StatusConflict,
	user.ErrCannotDeleteLastAdmin:   http.StatusConflict,
	user.ErrCannotDemoteSelf:        http.StatusConflict,
}

func MapError(err error) HTTPError {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	for targetErr, status := range errorRegistry {
		if errors.Is(err, targetErr) {
			return HTTPError{
				Status:  status,
				Message: err.Error(),
			}
		}
	}

	return HTTPError{
		Status:  http.StatusInternalServerError,
		Message: "An unexpected error occurred",
	}
}
