package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

const (
	CookieSession = "session_id"

	sessionTTL = 7 * 24 * time.Hour
)

type userKey struct{}

func userFromContext(r *http.Request) *user.User {
	u, _ := r.Context().Value(userKey{}).(*user.User)
	return u
}

// SetSession stores the session cookie for the user.
func SetSession(jar *cookies, w http.ResponseWriter, u *user.User) {
	jar.write(w, CookieSession, fmt.Sprintf("%s:%s", u.ID, security.Fingerprint(u.PasswordHash)), "/", sessionTTL)
}

// GetSession returns the session cookie's user id and fingerprint.
func GetSession(jar *cookies, r *http.Request) (id, fp string, ok bool) {
	val, ok := jar.read(r, CookieSession)
	if !ok {
		return "", "", false
	}
	id, fp, ok = strings.Cut(val, ":")
	return id, fp, ok
}

// ClearSession removes the session cookie.
func ClearSession(jar *cookies, w http.ResponseWriter) {
	jar.write(w, CookieSession, "", "/", -1)
}

// LoadSession loads the session user into the request context when a valid
// session cookie is present.
func LoadSession(jar *cookies, userSvc *user.UserService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id, fp, ok := GetSession(jar, r); ok {
				if u, err := userSvc.GetUser(id); err == nil && fp == security.Fingerprint(u.PasswordHash) {
					ctx := context.WithValue(r.Context(), userKey{}, u)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				ClearSession(jar, w)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireSession requires an authenticated user, otherwise redirects to login.
var RequireSession middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if userFromContext(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return nil
		}
		return next(w, r)
	}
}

// RequireAdmin requires an authenticated admin user.
var RequireAdmin middleware = func(next handler) handler {
	return RequireSession(func(w http.ResponseWriter, r *http.Request) error {
		if !userFromContext(r).IsAdmin {
			return ErrForbidden
		}
		return next(w, r)
	})
}

func (c *Controller) Index(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) LoginPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "login.html", nil)
}

func (c *Controller) LoginSubmit(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}

	u, err := c.userSvc.GetUserByUsername(r.PostForm.Get("username"))
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return ErrInvalidCredentials
		}
		return err
	}

	if !security.VerifyPassword(u.PasswordHash, r.PostForm.Get("password")) {
		return ErrInvalidCredentials
	}

	SetSession(c.cookies, w, u)
	c.cookies.SetFlash(w, "success", "logged in")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) Logout(w http.ResponseWriter, r *http.Request) error {
	ClearSession(c.cookies, w)
	c.cookies.SetFlash(w, "success", "logged out")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	return nil
}
