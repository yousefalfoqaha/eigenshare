package middleware

import (
	"net/http"

	"eigenshare/internal/security"
	"eigenshare/internal/session"
)

func WithSession(sessionSvc *session.SessionService, cookies *security.CookieManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token, ok := cookies.Get(r); ok {
				if u, err := sessionSvc.Validate(token); err == nil {
					next.ServeHTTP(w, r.WithContext(session.WithUser(r.Context(), u)))
					return
				}
				cookies.Clear(w)
			}
			next.ServeHTTP(w, r)
		})
	}
}

type SessionErrorHandler func(w http.ResponseWriter, r *http.Request)

func RequireSession(onError SessionErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := session.UserFromContext(r.Context()); u == nil {
				onError(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
