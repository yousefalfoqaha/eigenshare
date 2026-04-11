package api

import (
	"encoding/json"
	"net/http"
)

func MakeHandler(h controllerHandleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
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
