package web

import (
	"encoding/json"
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
	load     *dataLoader
	guard    *guards
}

func NewController(
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	secret []byte,
	secureCookies bool,
) *Controller {
	cookies := newCookies(secureCookies, secret)
	templs := newTemplates(cookies, userSvc)
	load := newDataLoader(cookies, userSvc, shareSvc)
	guard := newGuards(cookies)

	return &Controller{
		userSvc:  userSvc,
		shareSvc: shareSvc,
		cookies:  cookies,
		templs:   templs,
		load:     load,
		guard:    guard,
	}
}

func (c *Controller) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", c.HTMLRoutes())
	mux.Handle("/api/", c.APIRoutes())
	return logger(c.load.LoadSession(mux))
}

func (c *Controller) HTMLRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.templs.static)

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handleError(c.LoginPage))
	pub.Handle("POST /login", c.handleError(c.LoginSubmit))
	pub.Handle("POST /logout", c.handleError(c.Logout))
	pub.Handle("GET /s/{handle}", c.handleError(c.load.LoadShare(c.guard.Share(c.guard.UnlockedShare(c.SharePage)))))
	pub.Handle("GET /s/{handle}/unlock", c.handleError(c.load.LoadShare(c.UnlockPage)))
	pub.Handle("POST /s/{handle}/unlock", c.handleError(c.load.LoadShare(c.UnlockShare)))
	pub.Handle("GET /s/{handle}/download", c.handleError(c.load.LoadShare(c.guard.Share(c.guard.UnlockedShare(c.DownloadAll)))))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handleError(c.load.LoadShare(c.guard.Share(c.guard.UnlockedShare(c.DownloadFile)))))
	pub.Handle("GET /s/{handle}/uploads", c.handleError(c.load.LoadShare(c.guard.UnlockedShare(c.UploadRequestPage))))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handleError(c.guard.Session(c.Index)))
	prot.Handle("GET /shares", c.handleError(c.guard.Session(c.SharesPage)))
	prot.Handle("GET /shares/new", c.handleError(c.guard.Session(c.NewSharePage)))
	prot.Handle("POST /shares", c.handleError(c.guard.Session(c.CreateShare)))
	prot.Handle("GET /shares/{id}/edit", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.EditSharePage)))))
	prot.Handle("POST /shares/delete", c.handleError(c.guard.Session(c.DeleteShares)))
	prot.Handle("POST /shares/{id}/trash", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.TrashShare)))))
	prot.Handle("POST /shares/{id}/restore", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.RestoreShare)))))
	prot.Handle("POST /shares/{id}/visibility", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.SetVisibility)))))
	prot.Handle("POST /shares/{id}/security", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.UpdateShareSecurity)))))
	prot.Handle("POST /shares/{id}/owners", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.AddOwner)))))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.RemoveOwner)))))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.DeleteFile)))))
	prot.Handle("POST /shares/{id}/uploads", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.CreateUploadRequest)))))
	prot.Handle("POST /shares/{id}/uploads/{requestID}/revoke", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.RevokeUploadRequest)))))
	prot.Handle("GET /trash", c.handleError(c.guard.Session(c.TrashPage)))
	prot.Handle("GET /users", c.handleError(c.guard.Session(c.UsersPage)))

	prot.Handle("GET /users/new", c.handleError(c.guard.Admin(c.NewUserPage)))
	prot.Handle("POST /users", c.handleError(c.guard.Admin(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", c.handleError(c.guard.Admin(c.EditUserPage)))
	prot.Handle("POST /users/{id}", c.handleError(c.guard.Admin(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", c.handleError(c.guard.Admin(c.DeleteUser)))

	mux.Handle("/", prot)

	return mux
}

func (c *Controller) APIRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/shares/{id}/files", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.AddFilesToShare)))))
	mux.Handle("HEAD /api/shares/{id}/files/{fileID}/stream", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.GetUploadOffset)))))
	mux.Handle("PUT /api/shares/{id}/files/{fileID}/stream", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.StreamFile)))))
	mux.Handle("DELETE /api/shares/{id}/files/{fileID}", c.handleError(c.guard.Session(c.load.LoadShare(c.guard.ManageableShare(c.DeleteShareFile)))))

	mux.Handle("POST /api/s/{handle}/uploads/files", c.handleError(c.load.LoadShare(c.guard.UnlockedShare(c.AddFilesViaUploadRequest))))
	mux.Handle("HEAD /api/s/{handle}/uploads/files/{fileID}/stream", c.handleError(c.load.LoadShare(c.guard.UnlockedShare(c.GetUploadRequestFileOffset))))
	mux.Handle("PUT /api/s/{handle}/uploads/files/{fileID}/stream", c.handleError(c.load.LoadShare(c.guard.UnlockedShare(c.StreamUploadRequestFile))))
	mux.Handle("DELETE /api/s/{handle}/uploads/files/{fileID}", c.handleError(c.load.LoadShare(c.guard.UnlockedShare(c.DeleteUploadRequestFile))))

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

		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"message": msg})
			return
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
