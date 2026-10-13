package web

import (
	"context"
	"errors"
	"net/http"

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

// middleware mediates the request pipeline: it loads the session user
// and the route's share, renders auth pages, and enforces access
// policy. It carries exactly the dependencies those jobs need.
type middleware struct {
	cookies  *cookies
	userSvc  *user.UserService
	shareSvc *share.ShareService
}

func newMiddleware(cookies *cookies, userSvc *user.UserService, shareSvc *share.ShareService) *middleware {
	return &middleware{cookies: cookies, userSvc: userSvc, shareSvc: shareSvc}
}

// WithSession loads the user for a valid session cookie and stashes it
// in the request context. It never blocks a request.
func (m *middleware) WithSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := m.cookies.GetSession(r); ok {
			if u, err := m.userSvc.GetUser(id); err == nil && m.cookies.SessionValid(r, u) {
				ctx := context.WithValue(r.Context(), userKey{}, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// WithShare loads the share for the route's {handle} or {id} path value
// and stashes it in the request context. A share that doesn't exist
// slides through and is reported by the require guards; other failures
// propagate.
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

// RequireUnlockedShare redirects to the share's unlock form when a
// password-protected share has not been unlocked with a cookie issued
// against its current password.
func (m *middleware) RequireUnlockedShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return RequireShare(func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh.RequiresPassword(userFromContext(r)) && !m.cookies.HasUnlock(r, w, sh) {
			m.cookies.ClearUnlock(w, sh.ID)
			http.Redirect(w, r, "/s/"+sh.Handle+"/unlock", http.StatusSeeOther)
			return nil
		}
		return h(w, r)
	})
}

// RequireShare ensures a viewable share was stashed by WithShare.
func RequireShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanView(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return h(w, r)
	}
}

// RequireManageableShare ensures a manageable share was stashed by
// WithShare.
func RequireManageableShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanManage(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return h(w, r)
	}
}

// RequireSession redirects to the login page with 401 for
// unauthenticated requests.
func (m *middleware) RequireSession(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if userFromContext(r) == nil {
			m.cookies.ClearSession(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return nil
		}
		return h(w, r)
	}
}

// RequireAdmin additionally returns ErrForbidden for non-admin users;
// handleError renders it as a 403 error page.
func (m *middleware) RequireAdmin(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return m.RequireSession(func(w http.ResponseWriter, r *http.Request) error {
		if !userFromContext(r).IsAdmin {
			return ErrForbidden
		}
		return h(w, r)
	})
}
