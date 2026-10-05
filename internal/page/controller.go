package page

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"eigenshare/internal/middleware"
	"eigenshare/internal/security"
	"eigenshare/internal/session"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Controller struct {
	sessionSvc        *session.SessionService
	userSvc           *user.UserService
	shareSvc          *share.ShareService
	templs            map[string]*template.Template
	static            http.Handler
	sessionCookie     *security.CookieManager
	shareUnlockCookie *security.CookieManager
	shareUnlockSecret []byte
	flash             *flash
	bannedIps         map[string]time.Time
}

func NewController(
	authSvc *session.SessionService,
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	sessionCookie *security.CookieManager,
	shareUnlockCookie *security.CookieManager,
	shareUnlockSecret []byte,
	flashCookie *security.CookieManager,
) *Controller {
	funcs := template.FuncMap{
		"uploadedFiles":        uploadedFiles,
		"uploadedSize":         uploadedSize,
		"formatSize":           formatSize,
		"formatCommaSeparated": formatCommaSeparated,
		"expiresIn":            expiresIn,
		"timeAgo":              timeAgo,
		"dict":                 dict,
	}

	base := template.Must(template.New("layout").Funcs(funcs).
		ParseFS(templateFS, "templates/layout.html"))

	pageFiles, err := fs.Glob(templateFS, "templates/pages/*.html")
	if err != nil {
		log.Fatalf("failed to glob embedded templates: %v", err)
	}

	pages := make(map[string]*template.Template, len(pageFiles))
	for _, pagePath := range pageFiles {
		t := template.Must(base.Clone())
		template.Must(t.ParseFS(templateFS, pagePath))
		pages[filepath.Base(pagePath)] = t
	}

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to mount embedded static files: %v", err)
	}

	bannedIps := make(map[string]time.Time)

	return &Controller{
		sessionSvc:        authSvc,
		userSvc:           userSvc,
		shareSvc:          shareSvc,
		templs:            pages,
		static:            http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
		sessionCookie:     sessionCookie,
		shareUnlockCookie: shareUnlockCookie,
		shareUnlockSecret: shareUnlockSecret,
		flash:             newFlash(flashCookie),
		bannedIps:         bannedIps,
	}
}

func (c *Controller) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.static)

	requireSession := middleware.RequireSession(redirectToLogin)
	rateLimit := middleware.WithRateLimiting(c.bannedIps)
	requireAdmin := middleware.RequireAdmin(forbidden)

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handle(c.LoginPage))
	pub.Handle("POST /login", rateLimit(c.handle(c.LoginSubmit)))
	pub.Handle("POST /logout", c.handle(c.Logout))
	pub.Handle("GET /s/{handle}", c.handle(c.SharePage))
	pub.Handle("POST /s/{handle}/unlock", rateLimit(c.handle(c.UnlockShare)))
	pub.Handle("GET /s/{handle}/download", c.handle(c.DownloadAll))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handle(c.DownloadFile))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handle(c.Index))
	prot.Handle("GET /shares", c.handle(c.SharesPage))
	prot.Handle("GET /shares/new", c.handle(c.NewSharePage))
	prot.Handle("GET /shares/{id}/edit", c.handle(c.EditSharePage))
	prot.Handle("POST /shares/delete", c.handle(c.DeleteShares))
	prot.Handle("POST /shares/{id}/trash", c.handle(c.TrashShare))
	prot.Handle("POST /shares/{id}/restore", c.handle(c.RestoreShare))
	prot.Handle("POST /shares/{id}/visibility", c.handle(c.SetVisibility))
	prot.Handle("POST /shares/{id}/settings", c.handle(c.UpdateShareSettings))
	prot.Handle("POST /shares/{id}/owners", c.handle(c.AddOwner))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handle(c.RemoveOwner))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handle(c.DeleteFile))
	prot.Handle("GET /trash", c.handle(c.TrashPage))
	prot.Handle("GET /users", c.handle(c.UsersPage))

	prot.Handle("GET /users/new", requireAdmin(c.handle(c.NewUserPage)))
	prot.Handle("POST /users", requireAdmin(c.handle(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", requireAdmin(c.handle(c.EditUserPage)))
	prot.Handle("POST /users/{id}", requireAdmin(c.handle(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", requireAdmin(c.handle(c.DeleteUser)))

	mux.Handle("/", requireSession(prot))

	return mux
}

func (c *Controller) handle(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}

		status, msg := middleware.MapHTTPError(err)
		if status >= http.StatusInternalServerError {
			log.Printf("INTERNAL ERROR: %v", err)
		}

		if r.Method == http.MethodPost && status < http.StatusInternalServerError &&
			status != http.StatusNotFound && status != http.StatusForbidden {
			c.flash.Error(w, msg)
			http.Redirect(w, r, backURL(r, "/"), http.StatusSeeOther)
			return
		}

		_ = c.renderPage(w, r, status, "error.html", struct {
			Status  int
			Message string
		}{status, msg})
	}
}

func (c *Controller) renderPage(w http.ResponseWriter, r *http.Request, status int, tmpl string, content any) error {
	pg := pageData{
		Banner:      c.flash.Pop(w, r),
		CurrentUser: session.UserFromContext(r.Context()),
		Content:     content,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	return c.templs[tmpl].ExecuteTemplate(w, "layout", &pg)
}

func backURL(r *http.Request, fallback string) string {
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

type pageData struct {
	Banner      *banner
	CurrentUser *user.User
	Content     any
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
}

func forbidden(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}
