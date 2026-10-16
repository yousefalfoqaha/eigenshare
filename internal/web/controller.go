package web

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

// handler is any request handler that may fail.
type handler = func(http.ResponseWriter, *http.Request) error

// middleware wraps a handler.
type middleware = func(handler) handler

// compose collapses a middleware sequence into one middleware.
func compose(mws ...middleware) middleware {
	return func(h handler) handler {
		for _, mw := range slices.Backward(mws) {
			h = mw(h)
		}
		return h
	}
}

type Controller struct {
	userSvc  *user.UserService
	shareSvc *share.ShareService
	cookies  *cookies
	templs   *templates
}

func NewController(
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	secret []byte,
	secureCookies bool,
) *Controller {
	cookies := newCookies(secureCookies, secret)
	templs := newTemplates(cookies, userSvc)

	return &Controller{
		userSvc:  userSvc,
		shareSvc: shareSvc,
		cookies:  cookies,
		templs:   templs,
	}
}

func (c *Controller) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", c.HTMLRoutes())
	mux.Handle("/api/", c.APIRoutes())
	return logger(LoadSession(c.cookies, c.userSvc)(mux))
}

func (c *Controller) HTMLRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.templs.static)

	unlockShare := compose(LoadShare(c.shareSvc), RequireViewableShare)
	viewShare := compose(LoadShare(c.shareSvc), RequireViewableShare, RequireUnlockedShare(c.cookies))
	uploadLink := compose(LoadShare(c.shareSvc), RequireViewableShare, RequireUnlockedShare(c.cookies), LoadUploadRequest, RequireUploadRequest)

	protected := RequireSession
	admin := RequireAdmin

	editShare := compose(RequireSession, LoadShare(c.shareSvc), RequireManageable)

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handle(c.LoginPage))
	pub.Handle("POST /login", c.handle(c.LoginSubmit))
	pub.Handle("POST /logout", c.handle(c.Logout))
	pub.Handle("GET /s/{handle}", c.handle(viewShare(c.SharePage)))
	pub.Handle("GET /s/{handle}/unlock", c.handle(unlockShare(c.UnlockPage)))
	pub.Handle("POST /s/{handle}/unlock", c.handle(unlockShare(c.UnlockShare)))
	pub.Handle("GET /s/{handle}/download", c.handle(viewShare(c.DownloadAll)))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handle(viewShare(c.DownloadFile)))
	pub.Handle("GET /s/{handle}/uploads", c.handle(uploadLink(c.UploadRequestPage)))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handle(protected(c.Index)))
	prot.Handle("GET /shares", c.handle(protected(c.SharesPage)))
	prot.Handle("GET /shares/new", c.handle(protected(c.NewSharePage)))
	prot.Handle("POST /shares", c.handle(protected(c.CreateShare)))
	prot.Handle("GET /shares/{id}/edit", c.handle(editShare(c.EditSharePage)))
	prot.Handle("POST /shares/delete", c.handle(protected(c.DeleteShares)))
	prot.Handle("POST /shares/{id}/trash", c.handle(editShare(c.TrashShare)))
	prot.Handle("POST /shares/{id}/restore", c.handle(editShare(c.RestoreShare)))
	prot.Handle("POST /shares/{id}/visibility", c.handle(editShare(c.SetVisibility)))
	prot.Handle("POST /shares/{id}/security", c.handle(editShare(c.UpdateShareSecurity)))
	prot.Handle("POST /shares/{id}/owners", c.handle(editShare(c.AddOwner)))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handle(editShare(c.RemoveOwner)))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handle(editShare(c.DeleteFile)))
	prot.Handle("POST /shares/{id}/uploads", c.handle(editShare(c.CreateUploadRequest)))
	prot.Handle("POST /shares/{id}/uploads/{requestID}/revoke", c.handle(editShare(c.RevokeUploadRequest)))
	prot.Handle("GET /trash", c.handle(protected(c.TrashPage)))
	prot.Handle("GET /users", c.handle(protected(c.UsersPage)))

	prot.Handle("GET /users/new", c.handle(admin(c.NewUserPage)))
	prot.Handle("POST /users", c.handle(admin(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", c.handle(admin(c.EditUserPage)))
	prot.Handle("POST /users/{id}", c.handle(admin(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", c.handle(admin(c.DeleteUser)))

	mux.Handle("/", prot)

	return mux
}

func (c *Controller) APIRoutes() http.Handler {
	mux := http.NewServeMux()

	editShare := compose(RequireSession, LoadShare(c.shareSvc), RequireManageable)
	uploadLink := compose(LoadShare(c.shareSvc), RequireUnlockedShare(c.cookies), LoadUploadRequest, RequireUploadRequest)

	mux.Handle("POST /api/shares/{id}/files", c.handle(editShare(c.AddFilesToShare)))
	mux.Handle("HEAD /api/shares/{id}/files/{fileID}/stream", c.handle(editShare(c.GetUploadOffset)))
	mux.Handle("PUT /api/shares/{id}/files/{fileID}/stream", c.handle(editShare(c.StreamFile)))
	mux.Handle("DELETE /api/shares/{id}/files/{fileID}", c.handle(editShare(c.DeleteShareFile)))

	mux.Handle("POST /api/s/{handle}/uploads/files", c.handle(uploadLink(c.AddFilesViaUploadRequest)))
	mux.Handle("HEAD /api/s/{handle}/uploads/files/{fileID}/stream", c.handle(uploadLink(c.GetUploadRequestFileOffset)))
	mux.Handle("PUT /api/s/{handle}/uploads/files/{fileID}/stream", c.handle(uploadLink(c.StreamUploadRequestFile)))
	mux.Handle("DELETE /api/s/{handle}/uploads/files/{fileID}", c.handle(uploadLink(c.DeleteUploadRequestFile)))

	return mux
}

func (c *Controller) handle(h handler) http.HandlerFunc {
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

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, share.ErrInvalidRequest),
		errors.Is(err, share.ErrInvalidHandle),
		errors.Is(err, share.ErrInvalidFileName),
		errors.Is(err, share.ErrInvalidFileSize),
		errors.Is(err, share.ErrInvalidExpiry),
		errors.Is(err, share.ErrInvalidVisibility),
		errors.Is(err, share.ErrInvalidMaxBytes),
		errors.Is(err, security.ErrInvalidPassword),
		errors.Is(err, security.ErrPasswordTooLong),
		errors.Is(err, user.ErrInvalidUsername):
		return http.StatusBadRequest, err.Error()

	case errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, security.ErrWrongPassword):
		return http.StatusUnauthorized, err.Error()

	case errors.Is(err, share.ErrShareNotFound),
		errors.Is(err, share.ErrFileNotFound),
		errors.Is(err, share.ErrUploadRequestNotFound),
		errors.Is(err, user.ErrUserNotFound):
		return http.StatusNotFound, err.Error()

	case errors.Is(err, share.ErrHandleExists),
		errors.Is(err, share.ErrFileNotComplete),
		errors.Is(err, share.ErrInvalidChunkOffset),
		errors.Is(err, share.ErrChunkOffsetExceedsSize),
		errors.Is(err, share.ErrShareVersionConflict),
		errors.Is(err, share.ErrShareTrashed),
		errors.Is(err, share.ErrShareNotTrashed),
		errors.Is(err, share.ErrLastOwner),
		errors.Is(err, share.ErrOwnerExists),
		errors.Is(err, share.ErrUploadRequestLimit),
		errors.Is(err, user.ErrUsernameTaken),
		errors.Is(err, user.ErrVersionConflict),
		errors.Is(err, user.ErrCannotDeleteSelf),
		errors.Is(err, user.ErrCannotDeleteLastAdmin),
		errors.Is(err, user.ErrCannotDemoteSelf):
		return http.StatusConflict, err.Error()

	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, err.Error()

	case errors.Is(err, share.ErrUploadInProgress):
		return http.StatusLocked, err.Error()

	default:
		return http.StatusInternalServerError, "an unexpected error occurred"
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
