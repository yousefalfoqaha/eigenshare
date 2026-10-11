package web

import (
	"encoding/json"
	"net/http"

	"eigenshare/internal/session"
)

// WithSession validates the session cookie (if present) and injects the
// authenticated user into the request context.
func (c *Controller) WithSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := c.cookies.get(r, sessionCookieName); ok {
			if u, err := c.sessionSvc.Validate(token); err == nil {
				next.ServeHTTP(w, r.WithContext(session.WithUser(r.Context(), u)))
				return
			}
			c.cookies.clear(w, sessionCookieName)
		}
		next.ServeHTTP(w, r)
	})
}

type sessionErrorHandler func(w http.ResponseWriter, r *http.Request)

func requireSession(onError sessionErrorHandler) func(http.Handler) http.Handler {
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

func requireAdmin(onError sessionErrorHandler) func(http.Handler) http.Handler {
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

func writeAPIUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  http.StatusUnauthorized,
		"message": "unauthorized",
	})
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
}

func forbidden(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}
