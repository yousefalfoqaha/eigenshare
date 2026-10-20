package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"eigenshare/internal/security"
	"eigenshare/internal/user"
)

const (
	sessionCookie    = "session_id"
	sessionCookieTTL = 7 * 24 * time.Hour
)

type userKey struct{}

func userFromContext(r *http.Request) *user.User {
	u, _ := r.Context().Value(userKey{}).(*user.User)
	return u
}

func setSession(jar *cookies, w http.ResponseWriter, u *user.User) {
	jar.write(w, sessionCookie, fmt.Sprintf("%s:%s", u.ID, security.Fingerprint(u.PasswordHash)), "/", sessionCookieTTL)
}

func getSession(jar *cookies, r *http.Request) (id, fp string, ok bool) {
	val, ok := jar.read(r, sessionCookie)
	if !ok {
		return "", "", false
	}
	id, fp, ok = strings.Cut(val, ":")
	return id, fp, ok
}

func clearSession(jar *cookies, w http.ResponseWriter) {
	jar.write(w, sessionCookie, "", "/", -1)
}

func loadSession(jar *cookies, userSvc *user.UserService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id, fp, ok := getSession(jar, r); ok {
				if u, err := userSvc.GetUser(id); err == nil && fp == security.Fingerprint(u.PasswordHash) {
					ctx := context.WithValue(r.Context(), userKey{}, u)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				clearSession(jar, w)
			}

			next.ServeHTTP(w, r)
		})
	}
}

var requireSession middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if userFromContext(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return nil
		}
		return next(w, r)
	}
}

var requireAdmin middleware = func(next handler) handler {
	return requireSession(func(w http.ResponseWriter, r *http.Request) error {
		if !userFromContext(r).IsAdmin {
			return errForbidden
		}
		return next(w, r)
	})
}

func (c *Controller) index(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) loginPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "login.html", nil)
}

func (c *Controller) loginSubmit(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return errInvalidRequest
	}

	u, err := c.userSvc.GetUserByUsername(r.PostForm.Get("username"))
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return errInvalidCredentials
		}
		return err
	}

	if !security.VerifyPassword(u.PasswordHash, r.PostForm.Get("password")) {
		return errInvalidCredentials
	}

	setSession(c.cookies, w, u)
	c.cookies.SetFlash(w, "success", "logged in")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) logout(w http.ResponseWriter, r *http.Request) error {
	clearSession(c.cookies, w)
	c.cookies.SetFlash(w, "success", "logged out")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	return nil
}
