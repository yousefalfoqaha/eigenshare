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

type (
	handler    = func(http.ResponseWriter, *http.Request) error
	middleware = func(handler) handler
)

func compose(mws ...middleware) middleware {
	return func(h handler) handler {
		for _, mw := range slices.Backward(mws) {
			h = mw(h)
		}
		return h
	}
}

type Controller struct {
	userSvc     *user.UserService
	shareSvc    *share.ShareService
	cookies     *cookies
	templs      *templates
	chunkSizeMB int
}

func NewController(
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	secret []byte,
	secureCookies bool,
	chunkSizeMB int,
	demo bool,
) *Controller {
	cookies := newCookies(secureCookies, secret)
	templs := newTemplates(cookies, userSvc, demo)

	return &Controller{
		userSvc:     userSvc,
		shareSvc:    shareSvc,
		cookies:     cookies,
		templs:      templs,
		chunkSizeMB: chunkSizeMB,
	}
}

func (c *Controller) PageRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.templs.static)

	unlockShareChain := compose(loadShare(c.shareSvc), requireViewableShare)
	viewShareChain := compose(loadShare(c.shareSvc), requireViewableShare, requireUnlockedShare(c.cookies))
	uploadLinkChain := compose(loadShare(c.shareSvc), requireViewableShare, requireUnlockedShare(c.cookies), loadUploadLink, requireUploadLink)

	protectedChain := requireSession
	adminChain := requireAdmin

	editShareChain := compose(requireSession, loadShare(c.shareSvc), requireManageableShare)

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handle(c.loginPage))
	pub.Handle("POST /login", c.handle(c.loginSubmit))
	pub.Handle("POST /logout", c.handle(c.logout))
	pub.Handle("GET /s/{handle}", c.handle(viewShareChain(c.sharePage)))
	pub.Handle("GET /s/{handle}/unlock", c.handle(unlockShareChain(c.unlockPage)))
	pub.Handle("POST /s/{handle}/unlock", c.handle(unlockShareChain(c.unlockShare)))
	pub.Handle("GET /s/{handle}/download", c.handle(viewShareChain(c.downloadAll)))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handle(viewShareChain(c.downloadFile)))
	pub.Handle("GET /s/{handle}/uploads", c.handle(uploadLinkChain(c.uploadLinkPage)))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handle(protectedChain(c.index)))
	prot.Handle("GET /shares", c.handle(protectedChain(c.sharesPage)))
	prot.Handle("GET /shares/new", c.handle(protectedChain(c.newSharePage)))
	prot.Handle("POST /shares", c.handle(protectedChain(c.createShare)))
	prot.Handle("GET /shares/{id}/edit", c.handle(editShareChain(c.editSharePage)))
	prot.Handle("POST /shares/delete", c.handle(protectedChain(c.deleteShares)))
	prot.Handle("POST /shares/{id}/trash", c.handle(editShareChain(c.trashShare)))
	prot.Handle("POST /shares/{id}/restore", c.handle(editShareChain(c.restoreShare)))
	prot.Handle("POST /shares/{id}/visibility", c.handle(editShareChain(c.setVisibility)))
	prot.Handle("POST /shares/{id}/security", c.handle(editShareChain(c.updateShareSecurity)))
	prot.Handle("POST /shares/{id}/owners", c.handle(editShareChain(c.addOwner)))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handle(editShareChain(c.removeOwner)))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handle(editShareChain(c.deleteFile)))
	prot.Handle("POST /shares/{id}/uploads", c.handle(editShareChain(c.createUploadLink)))
	prot.Handle("POST /shares/{id}/uploads/{linkID}/revoke", c.handle(editShareChain(c.revokeUploadLink)))
	prot.Handle("GET /trash", c.handle(protectedChain(c.trashPage)))
	prot.Handle("GET /users", c.handle(protectedChain(c.usersPage)))

	prot.Handle("GET /users/new", c.handle(adminChain(c.newUserPage)))
	prot.Handle("POST /users", c.handle(adminChain(c.createUser)))
	prot.Handle("GET /users/{id}/edit", c.handle(adminChain(c.editUserPage)))
	prot.Handle("POST /users/{id}", c.handle(adminChain(c.updateUser)))
	prot.Handle("POST /users/{id}/delete", c.handle(adminChain(c.deleteUser)))

	mux.Handle("/", prot)

	return loadSession(c.cookies, c.userSvc)(mux)
}

func (c *Controller) APIRoutes() http.Handler {
	mux := http.NewServeMux()

	editShareChain := compose(requireSession, loadShare(c.shareSvc), requireManageableShare)
	uploadLinkChain := compose(loadShare(c.shareSvc), requireUnlockedShare(c.cookies), loadUploadLink, requireUploadLink)

	mux.Handle("POST /api/shares/{id}/files", c.handle(editShareChain(c.addFilesToShare)))
	mux.Handle("HEAD /api/shares/{id}/files/{fileID}/stream", c.handle(editShareChain(c.getUploadOffset)))
	mux.Handle("PUT /api/shares/{id}/files/{fileID}/stream", c.handle(editShareChain(c.streamFile)))
	mux.Handle("DELETE /api/shares/{id}/files/{fileID}", c.handle(editShareChain(c.deleteShareFile)))

	mux.Handle("POST /api/s/{handle}/uploads/files", c.handle(uploadLinkChain(c.addFilesViaUploadLink)))
	mux.Handle("HEAD /api/s/{handle}/uploads/files/{fileID}/stream", c.handle(uploadLinkChain(c.getUploadLinkFileOffset)))
	mux.Handle("PUT /api/s/{handle}/uploads/files/{fileID}/stream", c.handle(uploadLinkChain(c.streamUploadLinkFile)))
	mux.Handle("DELETE /api/s/{handle}/uploads/files/{fileID}", c.handle(uploadLinkChain(c.deleteUploadLinkFile)))

	return loadSession(c.cookies, c.userSvc)(mux)
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
	case errors.Is(err, errInvalidRequest),
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

	case errors.Is(err, errInvalidCredentials),
		errors.Is(err, security.ErrWrongPassword):
		return http.StatusUnauthorized, err.Error()

	case errors.Is(err, share.ErrShareNotFound),
		errors.Is(err, share.ErrFileNotFound),
		errors.Is(err, share.ErrUploadLinkNotFound),
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
		errors.Is(err, share.ErrUploadLinkLimit),
		errors.Is(err, share.ErrShareLimit),
		errors.Is(err, user.ErrUsernameTaken),
		errors.Is(err, user.ErrVersionConflict),
		errors.Is(err, user.ErrCannotDeleteSelf),
		errors.Is(err, user.ErrCannotDeleteLastAdmin),
		errors.Is(err, user.ErrCannotDemoteSelf):
		return http.StatusConflict, err.Error()

	case errors.Is(err, errForbidden):
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
