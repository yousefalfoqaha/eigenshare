package page

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"eigenshare/internal/auth"
	"eigenshare/internal/middleware"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Handler func(w http.ResponseWriter, r *http.Request) error

type Controller struct {
	Auth      *auth.AuthService
	User      *user.UserService
	Share     *share.ShareService
	templates map[string]*template.Template
	errorTmpl *template.Template
	static    http.Handler
}

func New(authSvc *auth.AuthService, userSvc *user.UserService, shareSvc *share.ShareService) *Controller {
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
		Auth:      authSvc,
		User:      userSvc,
		Share:     shareSvc,
		templates: pages,
		errorTmpl: errorTmpl,
		static:    http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
	}
}

func (c *Controller) StaticHandler() http.Handler {
	return c.static
}

func (c *Controller) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.StaticHandler())

	withAuth := middleware.WithAuth(redirectToLogin)
	withAdmin := middleware.WithAdmin(forbidden)

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handle(c.LoginPage))
	pub.Handle("POST /login", c.handle(c.LoginSubmit))
	pub.Handle("POST /logout", c.handle(c.Logout))
	pub.Handle("GET /s/{handle}", c.handle(c.SharePage))
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
	prot.Handle("POST /shares/{id}/owners", c.handle(c.AddOwner))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handle(c.RemoveOwner))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handle(c.DeleteFile))
	prot.Handle("GET /trash", c.handle(c.TrashPage))
	prot.Handle("GET /users", c.handle(c.UsersPage))

	prot.Handle("GET /users/new", withAdmin(c.handle(c.NewUserPage)))
	prot.Handle("POST /users", withAdmin(c.handle(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", withAdmin(c.handle(c.EditUserPage)))
	prot.Handle("POST /users/{id}", withAdmin(c.handle(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", withAdmin(c.handle(c.DeleteUser)))

	mux.Handle("/", withAuth(prot))

	return mux
}

func (c *Controller) handle(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			status, msg := middleware.MapError(err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			_ = c.errorTmpl.ExecuteTemplate(w, "error.html", struct {
				Status  int
				Message string
			}{status, msg})
		}
	}
}

type pageData struct {
	Banner      string
	CurrentUser *user.User
	Content     any
}

func (c *Controller) render(w http.ResponseWriter, r *http.Request, tmpl string, content any, domainErr error) error {
	banner := ""
	if domainErr != nil {
		_, banner = middleware.MapError(domainErr)
	}
	if banner == "" && r.URL.Query().Has("err") {
		banner = r.URL.Query().Get("err")
	}

	pg := pageData{
		Banner:      banner,
		CurrentUser: auth.UserFromContext(r.Context()),
		Content:     content,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return c.templates[tmpl].ExecuteTemplate(w, "layout", &pg)
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusFound)
}

func forbidden(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}
