package web

import (
	"eigenshare/backend/share"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

func MakeHandler(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			if apiErr, ok := shareAPIError(err); ok {
				JSON(w, apiErr.Status, apiErr)
			} else {
				log.Printf("ERROR: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}
	}
}

func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

func shareAPIError(err error) (Error, bool) {
	switch {
	case errors.Is(err, share.ErrInvalidRequest), errors.Is(err, share.ErrInvalidChunkOffset):
		return NewError(http.StatusBadRequest, err.Error()), true
	case errors.Is(err, share.ErrShareNotFound), errors.Is(err, share.ErrFileNotFound):
		return NewError(http.StatusNotFound, err.Error()), true
	case errors.Is(err, share.ErrHandleExists),
		errors.Is(err, share.ErrShareNotComplete),
		errors.Is(err, share.ErrChunkOffsetExceedsSize):
		return NewError(http.StatusConflict, err.Error()), true
	default:
		return Error{}, false
	}
}
