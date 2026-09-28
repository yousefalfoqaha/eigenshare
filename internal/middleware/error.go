package middleware

import (
	"errors"
	"log"
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func MapError(err error) (int, string) {
	if _, ok := errors.AsType[*share.ChunkOffsetConflictError](err); ok {
		return http.StatusConflict, err.Error()
	}

	switch {
	// 400 bad request
	case errors.Is(err, share.ErrInvalidRequest),
		errors.Is(err, share.ErrInvalidHandle),
		errors.Is(err, share.ErrInvalidFileName),
		errors.Is(err, share.ErrInvalidFileSize),
		errors.Is(err, share.ErrInvalidExpiry),
		errors.Is(err, share.ErrInvalidVisibility),
		errors.Is(err, share.ErrInvalidChunkOffset),
		errors.Is(err, user.ErrInvalidPassword),
		errors.Is(err, user.ErrInvalidUsername),
		errors.Is(err, auth.ErrInvalidToken),
		errors.Is(err, auth.ErrInvalidUserID),
		errors.Is(err, auth.ErrInvalidExpiry):
		return http.StatusBadRequest, err.Error()

	// 401 unauthorized
	case errors.Is(err, auth.ErrInvalidCredentials),
		errors.Is(err, auth.ErrSessionNotFound):
		return http.StatusUnauthorized, err.Error()

	// 404 not found
	case errors.Is(err, share.ErrShareNotFound),
		errors.Is(err, share.ErrFileNotFound),
		errors.Is(err, user.ErrUserNotFound):
		return http.StatusNotFound, err.Error()

	// 409 conflict
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

	// 423 locked
	case errors.Is(err, share.ErrUploadInProgress):
		return http.StatusLocked, err.Error()

	default:
		log.Printf("INTERNAL ERROR: %v", err)
		return http.StatusInternalServerError, "an unexpected error occurred"
	}
}
