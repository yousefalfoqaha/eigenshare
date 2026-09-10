package web

import (
	"encoding/json"
	"log"
	"net/http"
)

func writeAPIResponse(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

func makeAPIHandler(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			httpErr := MapError(err)

			if httpErr.Status == http.StatusInternalServerError {
				log.Printf("INTERNAL ERROR: %v", err)
			}

			_ = writeAPIResponse(w, httpErr.Status, httpErr)
		}
	}
}
