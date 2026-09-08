package web

import (
	"errors"
	"net/http"

	"eigenshare/backend/share"
)

type APIError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (e APIError) Error() string {
	return e.Message
}

func MapError(err error) APIError {
	if apiErr, ok := errors.AsType[APIError](err); ok {
		return apiErr
	}

	switch {
	case errors.Is(err, share.ErrInvalidRequest), errors.Is(err, share.ErrInvalidChunkOffset):
		return APIError{Status: http.StatusBadRequest, Message: err.Error()}

	case errors.Is(err, share.ErrShareNotFound), errors.Is(err, share.ErrFileNotFound):
		return APIError{Status: http.StatusNotFound, Message: err.Error()}

	case errors.Is(err, share.ErrHandleExists),
		errors.Is(err, share.ErrShareNotComplete),
		errors.Is(err, share.ErrChunkOffsetExceedsSize):
		return APIError{Status: http.StatusConflict, Message: err.Error()}
	}

	return APIError{
		Status:  http.StatusInternalServerError,
		Message: "An unexpected error occurred",
	}
}
