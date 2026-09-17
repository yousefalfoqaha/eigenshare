package app

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"eigenshare/internal/app/views"
	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type App struct {
	Auth   *auth.AuthService
	User   *user.UserService
	Share  *share.ShareService
	Pages  map[string]*template.Template
	Static http.Handler
}

type page struct {
	Title         string
	Error         string
	NotFound      bool
	CurrentUserID string
	Users         []*user.User
	Content       any
}

func (p *page) CurrentUser() *user.User {
	for _, u := range p.Users {
		if u.ID == p.CurrentUserID {
			return u
		}
	}
	return nil
}

func New(authSvc *auth.AuthService, userSvc *user.UserService, shareSvc *share.ShareService) *App {
	funcs := template.FuncMap{
		"shareTotalSize":  shareTotalSize,
		"formatSize":      formatSize,
		"shareTotalFiles": shareTotalFiles,
		"expiresIn":       expiresIn,
		"timeAgo":         timeAgo,
	}

	base := template.Must(template.New("layout").Funcs(funcs).
		ParseFS(templateFS, "templates/layout.html", "templates/components/*.html"))

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

	return &App{
		Auth:   authSvc,
		User:   userSvc,
		Share:  shareSvc,
		Pages:  pages,
		Static: http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
	}
}

func (app *App) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", app.Static)
	mux.HandleFunc("GET /login", app.loginPage)
	mux.HandleFunc("POST /login", app.loginSubmit)
	mux.HandleFunc("POST /logout", app.logout)

	pub := http.NewServeMux()
	pub.HandleFunc("GET /s/{handle}", app.sharePage)
	pub.HandleFunc("GET /s/{handle}/download", app.downloadAll)
	pub.HandleFunc("GET /s/{handle}/files/{fileID}/download", app.downloadFile)
	mux.Handle("/s/", app.OptionalAuth(pub))

	prot := http.NewServeMux()
	prot.HandleFunc("GET /", app.index)
	prot.HandleFunc("GET /shares", app.sharesPage)
	prot.HandleFunc("GET /shares/new", app.newSharePage)
	prot.HandleFunc("POST /shares", app.createShare)
	prot.HandleFunc("POST /shares/delete", app.deleteShares)
	prot.HandleFunc("GET /shares/{id}/edit", app.editSharePage)
	prot.HandleFunc("POST /shares/{id}/trash", app.trashShare)
	prot.HandleFunc("POST /shares/{id}/restore", app.restoreShare)
	prot.HandleFunc("POST /shares/{id}/files", app.addFilesToShare)
	prot.HandleFunc("POST /shares/{id}/files/{fileID}/delete", app.deleteFile)
	prot.HandleFunc("PUT /shares/{id}/files/{fileID}/stream", app.streamFile)
	prot.HandleFunc("POST /shares/{id}/owners", app.addOwner)
	prot.HandleFunc("POST /shares/{id}/owners/{ownerID}/delete", app.removeOwner)
	prot.HandleFunc("POST /shares/{id}/visibility", app.setVisibility)
	prot.HandleFunc("GET /trash", app.trashPage)
	prot.HandleFunc("GET /users", app.usersPage)
	admin := func(h http.HandlerFunc) http.Handler { return app.RequireAdmin(h) }
	prot.Handle("GET /users/new", admin(app.newUserPage))
	prot.Handle("POST /users", admin(app.createUser))
	prot.Handle("GET /users/{id}/edit", admin(app.editUserPage))
	prot.Handle("POST /users/{id}", admin(app.updateUser))
	prot.Handle("POST /users/{id}/delete", admin(app.deleteUser))
	mux.Handle("/", app.RequireAuth(prot))

	return app.Logging(mux)
}

func (app *App) render(w http.ResponseWriter, r *http.Request, template, title string, content any, err error) {
	t, ok := app.Pages[template]
	if !ok {
		log.Printf("INTERNAL ERROR: template not found: %s", template)
		app.clientError(w, r, fmt.Errorf("template not found: %s", template))
		return
	}

	status := http.StatusOK
	message := ""
	if err != nil {
		status, message = errorResponse(r, err)
	}

	if v, ok := content.(views.View); ok {
		if loadErr := v.Load(r); loadErr != nil {
			err = loadErr
			status, message = errorResponse(r, err)
		}
		title = v.Title()
	}

	page := page{
		Title:    title,
		Error:    message,
		NotFound: status == http.StatusNotFound,
		Content:  content,
	}
	if u := auth.UserFromContext(r.Context()); u != nil {
		page.CurrentUserID = u.ID
		users, err := app.User.GetAllUsers()
		if err != nil {
			log.Printf("INTERNAL ERROR: fetching users for page: %v (path: %s)", err, r.URL.Path)
		} else {
			page.Users = users
		}
	}

	buf := new(bytes.Buffer)
	if execErr := t.ExecuteTemplate(buf, "layout", &page); execErr != nil {
		log.Printf("INTERNAL ERROR: executing template %s: %v", template, execErr)
		app.clientError(w, r, execErr)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (app *App) clientError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := errorResponse(r, err)

	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": message})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, "<!doctype html><html><head><title>error - eigenshare</title></head><body><h1>%d</h1><p>%s</p></body></html>", status, message)
}

func errorResponse(r *http.Request, err error) (int, string) {
	status := http.StatusInternalServerError
	var statusErr interface{ HTTPStatus() int }
	if errors.As(err, &statusErr) {
		status = statusErr.HTTPStatus()
	}
	if status == http.StatusInternalServerError {
		log.Printf("INTERNAL ERROR: %v (path: %s)", err, r.URL.Path)
		return status, "An unexpected error occurred"
	}
	return status, err.Error()
}

func writeAPIResponse(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}
