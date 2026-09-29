package middleware

import (
	"net/http"

	"eigenshare/internal/session"
)

const cookieName = "session_id"

func GetSessionToken(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func SetSessionCookie(w http.ResponseWriter, token string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

type SessionErrorHandler func(w http.ResponseWriter, r *http.Request)

func WithSession(sessionSvc *session.SessionService, secureCookie bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
				if u, err := sessionSvc.Validate(cookie.Value); err == nil {
					next.ServeHTTP(w, r.WithContext(session.WithUser(r.Context(), u)))
					return
				}
				ClearSessionCookie(w, secureCookie)
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
