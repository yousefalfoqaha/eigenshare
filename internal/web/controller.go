package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"eigenshare/internal/session"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type banner struct {
	Kind    string
	Message string
}

type page struct {
	Banner        *banner
	CurrentUserID string
	Users         []*user.User
	Content       any
}

func (p *page) CurrentUser() *user.User {
	if p.CurrentUserID == "" {
		return nil
	}
	for _, u := range p.Users {
		if u.ID == p.CurrentUserID {
			return u
		}
	}
	return nil
}

type Controller struct {
	sessionSvc *session.SessionService
	userSvc    *user.UserService
	shareSvc   *share.ShareService
	templs     map[string]*template.Template
	static     http.Handler
	cookies    *cookies
}

func NewController(
	authSvc *session.SessionService,
	userSvc *user.UserService,
	shareSvc *share.ShareService,
	unlockSecret []byte,
	secureCookies bool,
) *Controller {
	funcs := template.FuncMap{
		"uploadedFiles": uploadedFiles,
		"uploadedSize":  uploadedSize,
		"formatSize":    formatSize,
		"formatOwners":  formatOwners,
		"expiresIn":     expiresIn,
		"timeAgo":       timeAgo,
		"dict":          dict,
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

	return &Controller{
		sessionSvc: authSvc,
		userSvc:    userSvc,
		shareSvc:   shareSvc,
		templs:     pages,
		static:     http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
		cookies:    newCookies(secureCookies, unlockSecret),
	}
}

func (c *Controller) HTMLRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", c.static)

	requireSession := requireSession(redirectToLogin)
	requireAdmin := requireAdmin(forbidden)

	pub := http.NewServeMux()
	pub.Handle("GET /login", c.handleHTML(c.LoginPage))
	pub.Handle("POST /login", c.handleHTML(c.LoginSubmit))
	pub.Handle("POST /logout", c.handleHTML(c.Logout))
	pub.Handle("GET /s/{handle}", c.handleHTML(c.SharePage))
	pub.Handle("POST /s/{handle}/unlock", c.handleHTML(c.UnlockShare))
	pub.Handle("GET /s/{handle}/download", c.handleHTML(c.DownloadAll))
	pub.Handle("GET /s/{handle}/files/{fileID}/download", c.handleHTML(c.DownloadFile))

	mux.Handle("/login", pub)
	mux.Handle("/logout", pub)
	mux.Handle("/s/", pub)

	prot := http.NewServeMux()
	prot.Handle("GET /", c.handleHTML(c.Index))
	prot.Handle("GET /shares", c.handleHTML(c.SharesPage))
	prot.Handle("GET /shares/new", c.handleHTML(c.NewSharePage))
	prot.Handle("POST /shares", c.handleHTML(c.CreateShare))
	prot.Handle("GET /shares/{id}/edit", c.handleHTML(c.EditSharePage))
	prot.Handle("POST /shares/delete", c.handleHTML(c.DeleteShares))
	prot.Handle("POST /shares/{id}/trash", c.handleHTML(c.TrashShare))
	prot.Handle("POST /shares/{id}/restore", c.handleHTML(c.RestoreShare))
	prot.Handle("POST /shares/{id}/visibility", c.handleHTML(c.SetVisibility))
	prot.Handle("POST /shares/{id}/security", c.handleHTML(c.UpdateShareSecurity))
	prot.Handle("POST /shares/{id}/owners", c.handleHTML(c.AddOwner))
	prot.Handle("POST /shares/{id}/owners/{ownerID}/delete", c.handleHTML(c.RemoveOwner))
	prot.Handle("POST /shares/{id}/files/{fileID}/delete", c.handleHTML(c.DeleteFile))
	prot.Handle("GET /trash", c.handleHTML(c.TrashPage))
	prot.Handle("GET /users", c.handleHTML(c.UsersPage))

	prot.Handle("GET /users/new", requireAdmin(c.handleHTML(c.NewUserPage)))
	prot.Handle("POST /users", requireAdmin(c.handleHTML(c.CreateUser)))
	prot.Handle("GET /users/{id}/edit", requireAdmin(c.handleHTML(c.EditUserPage)))
	prot.Handle("POST /users/{id}", requireAdmin(c.handleHTML(c.UpdateUser)))
	prot.Handle("POST /users/{id}/delete", requireAdmin(c.handleHTML(c.DeleteUser)))

	mux.Handle("/", requireSession(prot))

	return mux
}

func (c *Controller) APIRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /shares/{id}/files", handleAPI(c.AddFilesToShare))
	mux.Handle("HEAD /shares/{id}/files/{fileID}/stream", handleAPI(c.GetUploadOffset))
	mux.Handle("PUT /shares/{id}/files/{fileID}/stream", handleAPI(c.StreamFile))
	mux.Handle("DELETE /shares/{id}/files/{fileID}", handleAPI(c.DeleteShareFile))

	return requireSession(writeAPIUnauthorized)(mux)
}

func handleAPI(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			status, msg := mapError(err)
			if status >= http.StatusInternalServerError {
				log.Printf("INTERNAL ERROR: %v", err)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": msg})
		}
	}
}

func (c *Controller) handleHTML(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
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
			c.cookies.set(w, "flash", "error:"+msg)
			http.Redirect(w, r, backURL(r), http.StatusSeeOther)
			return
		}

		if err := c.renderPage(w, r, status, "error.html", struct {
			Status  int
			Message string
		}{status, msg}); err != nil {
			http.Error(w, msg, status)
		}
	}
}

func (c *Controller) renderPage(w http.ResponseWriter, r *http.Request, status int, tmpl string, content any) error {
	pg := page{
		Content: content,
	}

	if value, ok := c.cookies.get(r, "flash"); ok {
		c.cookies.clear(w, "flash")

		kind, raw, found := strings.Cut(value, ":")
		msg := raw
		if found {
			if unescaped, err := url.QueryUnescape(raw); err == nil {
				msg = unescaped
			}
		} else {
			kind, msg = "error", value
		}
		pg.Banner = &banner{Kind: kind, Message: msg}
	}

	if u := actor(r); u != nil {
		pg.CurrentUserID = u.ID
		users, err := c.userSvc.GetAllUsers()
		if err != nil {
			return err
		}
		pg.Users = users
	}

	var buf bytes.Buffer
	if err := c.templs[tmpl].ExecuteTemplate(&buf, "layout", &pg); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
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
