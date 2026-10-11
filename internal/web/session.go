package web

import (
	"context"
	"encoding/json"
	"net/http"

	"eigenshare/internal/user"
)

// userKey is the context key under which WithSession stores the
// authenticated user.
type userKey struct{}

func withUser(ctx context.Context, u *user.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// actor returns the authenticated user attached by WithSession, or nil for
// anonymous visitors. Handlers treat nil as "nobody is acting".
func actor(r *http.Request) *user.User {
	u, _ := r.Context().Value(userKey{}).(*user.User)
	return u
}

// WithSession validates the session cookie (if present) and injects the
// authenticated user into the request context.
func (c *Controller) WithSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, ok := c.cookies.get(r, "session"); ok {
			if u, err := c.sessionSvc.Validate(token); err == nil {
				next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
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
