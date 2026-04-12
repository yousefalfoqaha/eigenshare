package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

func NewError(status int, message string, cause error) Error {
	return Error{Status: status, Message: message, Cause: cause}
}

func MakeHandler(h controllerHandleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			if apiErr, ok := errors.AsType[Error](err); ok {
				if apiErr.Cause != nil {
					log.Printf("ERROR %d %s %s: %v", apiErr.Status, r.Method, r.URL.Path, apiErr.Cause)
				}
				w.WriteHeader(apiErr.Status)
			} else {
				log.Printf("ERROR %s %s: %v", r.Method, r.URL.Path, err)
				w.WriteHeader(http.StatusInternalServerError)
			}

			w.Header().Set("Content-Type", "application/json")
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			enc.Encode(err)
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
