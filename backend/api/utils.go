package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

func NewError(status int, message string) Error {
	return Error{Status: status, Message: message}
}

func MakeHandler(h controllerHandleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			if apiErr, ok := errors.AsType[Error](err); ok {
				JSON(w, apiErr.Status, err)
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
