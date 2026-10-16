package web

import (
	"net/http"
	"net/url"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
)

// guards enforce access invariants on context loaded by the dataLoader.
type guards struct {
	cookies *cookies
}

func newGuards(cookies *cookies) *guards {
	return &guards{cookies: cookies}
}

// Session requires an authenticated user, otherwise redirects to login.
func (g *guards) Session(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if userFromContext(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return nil
		}
		return h(w, r)
	}
}

// Admin requires an authenticated admin user.
func (g *guards) Admin(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return g.Session(func(w http.ResponseWriter, r *http.Request) error {
		if !userFromContext(r).IsAdmin {
			return ErrForbidden
		}
		return h(w, r)
	})
}

// Share requires the loaded share to be viewable.
func (g *guards) Share(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanView(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return h(w, r)
	}
}

// ManageableShare requires the loaded share to be manageable.
func (g *guards) ManageableShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanManage(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return h(w, r)
	}
}

// UnlockedShare requires a password-protected share to be unlocked,
// otherwise redirects to the unlock page preserving the original URL.
// Visibility is not checked here; routes that need it compose Share.
func (g *guards) UnlockedShare(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil {
			return share.ErrShareNotFound
		}
		if !sh.RequiresPassword(userFromContext(r)) {
			return h(w, r)
		}

		id, fp, ok := g.cookies.GetUnlock(r, sh.ID)
		if ok && id == sh.ID && fp == security.Fingerprint(sh.PasswordHash) {
			return h(w, r)
		}

		if ok {
			g.cookies.ClearUnlock(w, sh.ID)
		}

		http.Redirect(w, r, "/s/"+sh.Handle+"/unlock?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return nil
	}
}
