package pages

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

func (c *Controller) Wrap(h Handler) http.HandlerFunc {
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

func (c *Controller) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(middleware.SessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		u, err := c.Auth.ValidateSession(cookie.Value)
		if err != nil {
			middleware.ClearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

func (c *Controller) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil && cookie.Value != "" {
			if u, err := c.Auth.ValidateSession(cookie.Value); err == nil {
				next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (c *Controller) Admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := auth.UserFromContext(r.Context())
		if u == nil || !u.IsAdmin {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
