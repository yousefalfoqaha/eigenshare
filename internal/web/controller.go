package web

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type Controller struct {
	userSvc  *user.UserService
	shareSvc *share.ShareService
	cookies  *cookies
	templs   *templates
	mw       *middleware
}

func NewController(
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	secret []byte,
	secureCookies bool,
) *Controller {
	cookies := newCookies(secureCookies, security.NewSigner(secret))
	templs := newTemplates(cookies, userSvc)
	mw := newMiddleware(cookies, userSvc, shareSvc)

	return &Controller{
		userSvc:  userSvc,
		shareSvc: shareSvc,
		cookies:  cookies,
		templs:   templs,
		mw:       mw,
	}
}

// Handler returns the fully wired application handler: session loading,
// request logging, and route dispatch.
func (c *Controller) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", c.HTMLRoutes())
	mux.Handle("/api/", c.APIRoutes())
	return logger(c.mw.WithSession(mux))
}

func (c *Controller) HTMLRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.templs.static)

	mw := c.mw

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handleError(c.LoginPage))
	pub.Handle("POST /login", c.handleError(c.LoginSubmit))
	pub.Handle("POST /logout", c.handleError(c.Logout))
	pub.Handle("GET /s/{handle}", c.handleError(mw.WithShare(mw.RequireUnlockedShare(c.SharePage))))
	pub.Handle("GET /s/{handle}/unlock", c.handleError(mw.WithShare(RequireShare(c.UnlockPage))))
	pub.Handle("POST /s/{handle}/unlock", c.handleError(mw.WithShare(RequireShare(c.UnlockShare))))
	pub.Handle("GET /s/{handle}/download", c.handleError(mw.WithShare(mw.RequireUnlockedShare(c.DownloadAll))))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handleError(mw.WithShare(mw.RequireUnlockedShare(c.DownloadFile))))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handleError(mw.RequireSession(c.Index)))
	prot.Handle("GET /shares", c.handleError(mw.RequireSession(c.SharesPage)))
	prot.Handle("GET /shares/new", c.handleError(mw.RequireSession(c.NewSharePage)))
	prot.Handle("POST /shares", c.handleError(mw.RequireSession(c.CreateShare)))
	prot.Handle("GET /shares/{id}/edit", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.EditSharePage)))))
	prot.Handle("POST /shares/delete", c.handleError(mw.RequireSession(c.DeleteShares)))
	prot.Handle("POST /shares/{id}/trash", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.TrashShare)))))
	prot.Handle("POST /shares/{id}/restore", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.RestoreShare)))))
	prot.Handle("POST /shares/{id}/visibility", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.SetVisibility)))))
	prot.Handle("POST /shares/{id}/security", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.UpdateShareSecurity)))))
	prot.Handle("POST /shares/{id}/owners", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.AddOwner)))))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.RemoveOwner)))))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.DeleteFile)))))
	prot.Handle("GET /trash", c.handleError(mw.RequireSession(c.TrashPage)))
	prot.Handle("GET /users", c.handleError(mw.RequireSession(c.UsersPage)))

	prot.Handle("GET /users/new", c.handleError(mw.RequireAdmin(c.NewUserPage)))
	prot.Handle("POST /users", c.handleError(mw.RequireAdmin(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", c.handleError(mw.RequireAdmin(c.EditUserPage)))
	prot.Handle("POST /users/{id}", c.handleError(mw.RequireAdmin(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", c.handleError(mw.RequireAdmin(c.DeleteUser)))

	mux.Handle("/", prot)

	return mux
}

func (c *Controller) APIRoutes() http.Handler {
	mux := http.NewServeMux()
	mw := c.mw

	mux.Handle("POST /api/shares/{id}/files", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.AddFilesToShare)))))
	mux.Handle("HEAD /api/shares/{id}/files/{fileID}/stream", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.GetUploadOffset)))))
	mux.Handle("PUT /api/shares/{id}/files/{fileID}/stream", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.StreamFile)))))
	mux.Handle("DELETE /api/shares/{id}/files/{fileID}", c.handleError(mw.RequireSession(mw.WithShare(RequireManageableShare(c.DeleteShareFile)))))

	return mux
}

// handleError wraps route handlers: errors are mapped, logged when
// severe, and answered as a flash-redirect (POST) or an error page.
func (c *Controller) handleError(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}

		status, msg := mapError(err)
		if status >= http.StatusInternalServerError {
			log.Printf("INTERNAL ERROR: %v", err)
		}

		if r.Method == http.MethodPost && status < http.StatusInternalServerError &&
			status != http.StatusNotFound && status != http.StatusForbidden {
			c.cookies.SetFlash(w, "error", msg)
			http.Redirect(w, r, backURL(r), http.StatusSeeOther)
			return
		}

		if err := c.templs.renderPage(w, r, status, "error.html", struct {
			Status  int
			Message string
		}{status, msg}); err != nil {
			http.Error(w, msg, status)
		}
	}
}

func backURL(r *http.Request) string {
	fallback := "/"
	ref := r.Referer()
	if ref == "" {
		return fallback
	}

	u, err := url.Parse(ref)
	if err != nil || u.Host != r.Host || u.Path == "" || strings.HasPrefix(u.Path, "//") {
		return fallback
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}
