package web

import (
	"log"
	"net/http"
	"net/url"
	"strings"

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
	cookies := newCookies(secureCookies, secret)
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
	pub.Handle("GET /s/{handle}", c.handleError(mw.WithShare(mw.requireUnlockedShare(c.SharePage))))
	pub.Handle("GET /s/{handle}/unlock", c.handleError(mw.WithShare(requireShare(c.UnlockPage))))
	pub.Handle("POST /s/{handle}/unlock", c.handleError(mw.WithShare(requireShare(c.UnlockShare))))
	pub.Handle("GET /s/{handle}/download", c.handleError(mw.WithShare(mw.requireUnlockedShare(c.DownloadAll))))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handleError(mw.WithShare(mw.requireUnlockedShare(c.DownloadFile))))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handleError(requireSession(c.Index)))
	prot.Handle("GET /shares", c.handleError(requireSession(c.SharesPage)))
	prot.Handle("GET /shares/new", c.handleError(requireSession(c.NewSharePage)))
	prot.Handle("POST /shares", c.handleError(requireSession(c.CreateShare)))
	prot.Handle("GET /shares/{id}/edit", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.EditSharePage)))))
	prot.Handle("POST /shares/delete", c.handleError(requireSession(c.DeleteShares)))
	prot.Handle("POST /shares/{id}/trash", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.TrashShare)))))
	prot.Handle("POST /shares/{id}/restore", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.RestoreShare)))))
	prot.Handle("POST /shares/{id}/visibility", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.SetVisibility)))))
	prot.Handle("POST /shares/{id}/security", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.UpdateShareSecurity)))))
	prot.Handle("POST /shares/{id}/owners", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.AddOwner)))))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.RemoveOwner)))))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.DeleteFile)))))
	prot.Handle("GET /trash", c.handleError(requireSession(c.TrashPage)))
	prot.Handle("GET /users", c.handleError(requireSession(c.UsersPage)))

	prot.Handle("GET /users/new", c.handleError(requireAdminSession(c.NewUserPage)))
	prot.Handle("POST /users", c.handleError(requireAdminSession(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", c.handleError(requireAdminSession(c.EditUserPage)))
	prot.Handle("POST /users/{id}", c.handleError(requireAdminSession(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", c.handleError(requireAdminSession(c.DeleteUser)))

	mux.Handle("/", prot)

	return mux
}

func (c *Controller) APIRoutes() http.Handler {
	mux := http.NewServeMux()
	mw := c.mw

	mux.Handle("POST /api/shares/{id}/files", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.AddFilesToShare)))))
	mux.Handle("HEAD /api/shares/{id}/files/{fileID}/stream", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.GetUploadOffset)))))
	mux.Handle("PUT /api/shares/{id}/files/{fileID}/stream", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.StreamFile)))))
	mux.Handle("DELETE /api/shares/{id}/files/{fileID}", c.handleError(requireSession(mw.WithShare(requireManageableShare(c.DeleteShareFile)))))

	return mux
}

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
