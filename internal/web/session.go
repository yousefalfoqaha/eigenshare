package web

import (
	"context"
	"encoding/json"
	"net/http"

	"eigenshare/internal/user"
)

type userKey struct{}

func actor(r *http.Request) *user.User {
	u, _ := r.Context().Value(userKey{}).(*user.User)
	return u
}

func (c *Controller) WithSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := c.cookies.get(r, "session"); ok {
			if u, err := c.sessionSvc.Validate(token); err == nil {
				ctx := context.WithValue(r.Context(), userKey{}, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			c.cookies.clear(w, "session")
		}
		next.ServeHTTP(w, r)
	})
}

type sessionErrorHandler func(w http.ResponseWriter, r *http.Request)

func requireSession(onError sessionErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if actor(r) == nil {
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
			u := actor(r)
			if u == nil || !u.IsAdmin {
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
