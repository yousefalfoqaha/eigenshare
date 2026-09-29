package middleware

import (
	"net/http"

	"eigenshare/internal/auth"
)

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

type AuthErrorHandler func(w http.ResponseWriter, r *http.Request)

func WithUser(authSvc *auth.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
				if u, err := authSvc.ValidateSession(cookie.Value); err == nil {
					next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
					return
				}
				ClearSessionCookie(w)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func WithAuth(onError AuthErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := auth.UserFromContext(r.Context()); u == nil {
				onError(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func WithAdmin(onError AuthErrorHandler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := auth.UserFromContext(r.Context())
			if !u.IsAdmin {
				onError(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
