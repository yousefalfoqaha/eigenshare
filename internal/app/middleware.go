package app

import (
	"log"
	"net/http"
	"strings"
	"time"

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

func (app *App) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(SessionCookieName)
		if err != nil || c.Value == "" {
			deny(w, r)
			return
		}

		u, err := app.Auth.ValidateSession(c.Value)
		if err != nil {
			ClearSessionCookie(w)
			deny(w, r)
			return
		}

		ctx := auth.WithUser(r.Context(), u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (app *App) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
			if u, err := app.Auth.ValidateSession(c.Value); err == nil {
				ctx := auth.WithUser(r.Context(), u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (app *App) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromContext(r.Context())
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

type WrappedWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *WrappedWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	w.status = status
	w.written = true
}

func (w *WrappedWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

func (app *App) Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrappedWriter := &WrappedWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		start := time.Now()

		next.ServeHTTP(wrappedWriter, r)

		log.Println(wrappedWriter.status, r.Method, r.URL.Path, time.Since(start))
	})
}
