package handler

import (
	"eigenshare/backend/api"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

func MakeHandler(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			if apiErr, ok := errors.AsType[api.Error](err); ok {
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
