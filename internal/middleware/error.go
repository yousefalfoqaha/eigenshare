package middleware

import (
	"errors"
	"log"
	"net/http"

	"eigenshare/internal/session"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type ErrorHandler func(w http.ResponseWriter, r *http.Request, status int, msg string)

func WithErrorHandler(onError ErrorHandler) func(func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				status, msg := mapError(err)
				onError(w, r, status, msg)
			}
		}
	}
}

func mapError(err error) (int, string) {
	if _, ok := errors.AsType[*share.ChunkOffsetConflictError](err); ok {
		return http.StatusConflict, err.Error()
	}

	switch {
	case errors.Is(err, share.ErrInvalidRequest),
		errors.Is(err, share.ErrInvalidHandle),
		errors.Is(err, share.ErrInvalidFileName),
		errors.Is(err, share.ErrInvalidFileSize),
		errors.Is(err, share.ErrInvalidExpiry),
		errors.Is(err, share.ErrInvalidVisibility),
		errors.Is(err, share.ErrInvalidChunkOffset),
		errors.Is(err, user.ErrInvalidPassword),
		errors.Is(err, user.ErrInvalidUsername),
		errors.Is(err, session.ErrInvalidToken),
		errors.Is(err, session.ErrInvalidUserID),
		errors.Is(err, session.ErrInvalidExpiry):
		return http.StatusBadRequest, err.Error()

	case errors.Is(err, session.ErrInvalidCredentials),
		errors.Is(err, session.ErrSessionNotFound):
		return http.StatusUnauthorized, err.Error()

	case errors.Is(err, share.ErrShareNotFound),
		errors.Is(err, share.ErrFileNotFound),
		errors.Is(err, user.ErrUserNotFound):
		return http.StatusNotFound, err.Error()

	case errors.Is(err, share.ErrHandleExists),
		errors.Is(err, share.ErrFileNotComplete),
		errors.Is(err, share.ErrChunkOffsetExceedsSize),
		errors.Is(err, share.ErrShareVersionConflict),
		errors.Is(err, share.ErrShareTrashed),
		errors.Is(err, share.ErrShareNotTrashed),
		errors.Is(err, share.ErrLastOwner),
		errors.Is(err, share.ErrOwnerExists),
		errors.Is(err, user.ErrUsernameTaken),
		errors.Is(err, user.ErrCannotDeleteSelf),
		errors.Is(err, user.ErrCannotDeleteLastAdmin),
		errors.Is(err, user.ErrCannotDemoteSelf):
		return http.StatusConflict, err.Error()

	case errors.Is(err, share.ErrUploadInProgress):
		return http.StatusLocked, err.Error()

	default:
		log.Printf("INTERNAL ERROR: %v", err)
		return http.StatusInternalServerError, "an unexpected error occurred"
	}
}
