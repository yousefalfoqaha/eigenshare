package web

import (
	"context"
	"errors"
	"net/http"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type userKey struct{}

func userFromContext(r *http.Request) *user.User {
	u, _ := r.Context().Value(userKey{}).(*user.User)
	return u
}

type shareKey struct{}

func shareFromContext(r *http.Request) *share.Share {
	sh, _ := r.Context().Value(shareKey{}).(*share.Share)
	return sh
}

// dataLoader loads request-scoped data into the request context. It never
// enforces access; guards do.
type dataLoader struct {
	cookies  *cookies
	userSvc  *user.UserService
	shareSvc *share.ShareService
}

func newDataLoader(cookies *cookies, userSvc *user.UserService, shareSvc *share.ShareService) *dataLoader {
	return &dataLoader{cookies: cookies, userSvc: userSvc, shareSvc: shareSvc}
}

// LoadSession loads the session user into the request context when a valid
// session cookie is present.
func (l *dataLoader) LoadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, fp, ok := l.cookies.GetSession(r); ok {
			if u, err := l.userSvc.GetUser(id); err == nil && fp == security.Fingerprint(u.PasswordHash) {
				ctx := context.WithValue(r.Context(), userKey{}, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			l.cookies.ClearSession(w)
		}

		next.ServeHTTP(w, r)
	})
}

// LoadShare loads the share aggregate identified by the {handle} or {id}
// path value into the request context. Shares that cannot be found are left
// nil so guards can decide how to respond.
func (l *dataLoader) LoadShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var (
			sh  *share.Share
			err error
		)
		if handle := r.PathValue("handle"); handle != "" {
			sh, err = l.shareSvc.GetShareByHandle(handle)
		} else if id := r.PathValue("id"); id != "" {
			sh, err = l.shareSvc.GetShare(id)
		}
		if err != nil {
			if !errors.Is(err, share.ErrShareNotFound) {
				return err
			}
			return h(w, r)
		}
		if sh == nil {
			return h(w, r)
		}

		ctx := context.WithValue(r.Context(), shareKey{}, sh)
		return h(w, r.WithContext(ctx))
	}
}
