package middleware

import (
	"net/http"

	"eigenshare/internal/session"
)

func RequireAdmin(onError SessionErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := session.UserFromContext(r.Context())
			if !u.IsAdmin {
				onError(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
