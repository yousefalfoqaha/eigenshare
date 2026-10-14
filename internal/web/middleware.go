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

type middleware struct {
	cookies  *cookies
	userSvc  *user.UserService
	shareSvc *share.ShareService
}

func newMiddleware(cookies *cookies, userSvc *user.UserService, shareSvc *share.ShareService) *middleware {
	return &middleware{cookies: cookies, userSvc: userSvc, shareSvc: shareSvc}
}

func (m *middleware) WithSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, fp, ok := m.cookies.GetSession(r); ok {
			if u, err := m.userSvc.GetUser(id); err == nil && fp == security.Fingerprint(u.PasswordHash) {
				ctx := context.WithValue(r.Context(), userKey{}, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			m.cookies.ClearSession(w)
		}

		next.ServeHTTP(w, r)
	})
}

func (m *middleware) WithShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var (
			sh  *share.Share
			err error
		)
		if handle := r.PathValue("handle"); handle != "" {
			sh, err = m.shareSvc.GetShareByHandle(handle)
		} else if id := r.PathValue("id"); id != "" {
			sh, err = m.shareSvc.GetShare(id)
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

func (m *middleware) requireUnlockedShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return requireShare(func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if !sh.RequiresPassword(userFromContext(r)) {
			return h(w, r)
		}

		id, fp, ok := m.cookies.GetUnlock(r, sh.ID)
		if ok && id == sh.ID && fp == security.Fingerprint(sh.PasswordHash) {
			return h(w, r)
		}

		if ok {
			m.cookies.ClearUnlock(w, sh.ID)
		}

		http.Redirect(w, r, "/s/"+sh.Handle+"/unlock", http.StatusSeeOther)
		return nil
	})
}

func requireShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanView(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return h(w, r)
	}
}

func requireManageableShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanManage(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return h(w, r)
	}
}

func requireSession(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if userFromContext(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return nil
		}
		return h(w, r)
	}
}

func requireAdminSession(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return requireSession(func(w http.ResponseWriter, r *http.Request) error {
		if !userFromContext(r).IsAdmin {
			return ErrForbidden
		}
		return h(w, r)
	})
}
