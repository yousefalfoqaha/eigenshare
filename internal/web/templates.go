package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

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

type templates struct {
	templs  map[string]*template.Template
	static  http.Handler
	cookies *cookies
	userSvc *user.UserService
}

func newTemplates(cookies *cookies, userSvc *user.UserService) *templates {
	funcs := template.FuncMap{
		"uploadedFiles": uploadedFiles,
		"uploadedSize":  uploadedSize,
		"formatSize":    formatSize,
		"formatOwners":  formatOwners,
		"expiresIn":     expiresIn,
		"timeAgo":       timeAgo,
		"dict":          dict,
		"jsonify": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},
	}

	base := template.Must(template.New("layout").Funcs(funcs).
		ParseFS(templateFS, "templates/layout.html", "templates/partials/uploader.html"))

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

	return &templates{
		templs:  pages,
		static:  http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
		cookies: cookies,
		userSvc: userSvc,
	}
}

func (t *templates) renderPage(w http.ResponseWriter, r *http.Request, status int, tmpl string, content any) error {
	pg := page{
		Content: content,
	}

	if kind, message, ok := t.cookies.GetFlash(r, w); ok {
		pg.Banner = &banner{Kind: kind, Message: message}
	}

	if u := userFromContext(r); u != nil {
		pg.CurrentUserID = u.ID
		users, err := t.userSvc.GetAllUsers()
		if err != nil {
			return err
		}
		pg.Users = users
	}

	var buf bytes.Buffer
	if err := t.templs[tmpl].ExecuteTemplate(&buf, "layout", &pg); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}
