package page

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

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
	errorTmpl         *template.Template
	static            http.Handler
	sessionCookie     *security.CookieManager
	shareUnlockCookie *security.CookieManager
	shareUnlockSecret []byte
}

func NewController(
	authSvc *session.SessionService,
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	sessionCookie *security.CookieManager,
	shareUnlockCookie *security.CookieManager,
	shareUnlockSecret []byte,
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

	errorTmpl := template.Must(template.New("error.html").Funcs(funcs).
		ParseFS(templateFS, "templates/error.html"))

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to mount embedded static files: %v", err)
	}

	return &Controller{
		sessionSvc:        authSvc,
		userSvc:           userSvc,
		shareSvc:          shareSvc,
		templs:            pages,
		errorTmpl:         errorTmpl,
		static:            http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
		sessionCookie:     sessionCookie,
		shareUnlockCookie: shareUnlockCookie,
		shareUnlockSecret: shareUnlockSecret,
	}
}

func (c *Controller) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.static)

	requireSession := middleware.RequireSession(redirectToLogin)
	requireAdmin := middleware.RequireAdmin(forbidden)
	handleErr := middleware.WithErrorHandler(c.renderError)

	pub := http.NewServeMux()
	pub.Handle("GET /login", handleErr(c.LoginPage))
	pub.Handle("POST /login", handleErr(c.LoginSubmit))
	pub.Handle("POST /logout", handleErr(c.Logout))
	pub.Handle("GET /s/{handle}", handleErr(c.SharePage))
	pub.Handle("POST /s/{handle}/unlock", handleErr(c.UnlockShare))
	pub.Handle("GET /s/{handle}/download", handleErr(c.DownloadAll))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", handleErr(c.DownloadFile))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", handleErr(c.Index))
	prot.Handle("GET /shares", handleErr(c.SharesPage))
	prot.Handle("GET /shares/new", handleErr(c.NewSharePage))
	prot.Handle("GET /shares/{id}/edit", handleErr(c.EditSharePage))
	prot.Handle("POST /shares/delete", handleErr(c.DeleteShares))
	prot.Handle("POST /shares/{id}/trash", handleErr(c.TrashShare))
	prot.Handle("POST /shares/{id}/restore", handleErr(c.RestoreShare))
	prot.Handle("POST /shares/{id}/visibility", handleErr(c.SetVisibility))
	prot.Handle("POST /shares/{id}/settings", handleErr(c.UpdateShareSettings))
	prot.Handle("POST /shares/{id}/owners", handleErr(c.AddOwner))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", handleErr(c.RemoveOwner))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", handleErr(c.DeleteFile))
	prot.Handle("GET /trash", handleErr(c.TrashPage))
	prot.Handle("GET /users", handleErr(c.UsersPage))

	prot.Handle("GET /users/new", requireAdmin(handleErr(c.NewUserPage)))
	prot.Handle("POST /users", requireAdmin(handleErr(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", requireAdmin(handleErr(c.EditUserPage)))
	prot.Handle("POST /users/{id}", requireAdmin(handleErr(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", requireAdmin(handleErr(c.DeleteUser)))

	mux.Handle("/", requireSession(prot))

	return mux
}

func (c *Controller) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = c.errorTmpl.ExecuteTemplate(w, "error.html", struct {
		Status  int
		Message string
	}{status, msg})
}

type pageData struct {
	Banner      string
	CurrentUser *user.User
	Content     any
}

func (c *Controller) render(w http.ResponseWriter, r *http.Request, tmpl string, content any, err error) error {
	banner := ""
	if err != nil {
		banner = err.Error()
	}
	if banner == "" && r.URL.Query().Has("err") {
		banner = r.URL.Query().Get("err")
	}

	pg := pageData{
		Banner:      banner,
		CurrentUser: session.UserFromContext(r.Context()),
		Content:     content,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return c.templs[tmpl].ExecuteTemplate(w, "layout", &pg)
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
}

func forbidden(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}
