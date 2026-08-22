package auth

import (
	"context"
	"eigenshare/backend/user"
	"net/http"
	"strings"
)

type userKey struct{}

func UserFromContext(ctx context.Context) *user.User {
	u, _ := ctx.Value(userKey{}).(*user.User)
	return u
}

const SessionCookieName = "session_id"

func SetSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func RequireAuth(svc *AuthService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(SessionCookieName)
		if err != nil || c.Value == "" {
			deny(w, r)
			return
		}

		u, err := svc.ValidateSession(c.Value)
		if err != nil {
			ClearSessionCookie(w)
			deny(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), userKey{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func OptionalAuth(svc *AuthService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
			if u, err := svc.ValidateSession(c.Value); err == nil {
				ctx := context.WithValue(r.Context(), userKey{}, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil || !u.IsAdmin {
			if wantsJSON(r) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"status":403,"message":"forbidden"}`))
				return
			}
			http.Redirect(w, r, "/shares", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func deny(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":401,"message":"unauthorized"}`))
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func wantsJSON(r *http.Request) bool {
	if accept := r.Header.Get("Accept"); strings.Contains(accept, "application/json") {
		return true
	}
	if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
		return true
	}
	if r.Method == http.MethodPut || r.Method == http.MethodDelete {
		return true
	}
	return false
}
