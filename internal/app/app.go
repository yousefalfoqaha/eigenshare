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

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type App struct {
	Auth      *auth.AuthService
	User      *user.UserService
	Share     *share.ShareService
	templates map[string]*template.Template
	static    http.Handler
}

func New(authSvc *auth.AuthService, userSvc *user.UserService, shareSvc *share.ShareService) *App {
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

	return &App{
		Auth:      authSvc,
		User:      userSvc,
		Share:     shareSvc,
		templates: pages,
		static:    http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
	}
}

func (app *App) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", app.static)

	pub := http.NewServeMux()
	pub.HandleFunc("GET /login", app.loginPage)
	pub.HandleFunc("POST /login", app.loginSubmit)
	pub.HandleFunc("POST /logout", app.logout)
	pub.HandleFunc("GET /s/{handle}", app.sharePage)
	pub.HandleFunc("GET /s/{handle}/download", app.downloadAll)
	pub.HandleFunc("GET /s/{handle}/files/{fileID}/download", app.downloadFile)
	mux.Handle("/login", app.optionalAuth(pub))
	mux.Handle("/logout", app.optionalAuth(pub))
	mux.Handle("/s/", app.optionalAuth(pub))

	prot := http.NewServeMux()
	prot.HandleFunc("GET /", app.index)
	prot.HandleFunc("GET /shares", app.sharesPage)
	prot.HandleFunc("GET /shares/new", app.newSharePage)
	prot.HandleFunc("GET /shares/{id}/edit", app.editSharePage)
	prot.HandleFunc("POST /shares/delete", app.deleteShares)
	prot.HandleFunc("POST /shares/{id}/trash", app.trashShare)
	prot.HandleFunc("POST /shares/{id}/restore", app.restoreShare)
	prot.HandleFunc("POST /shares/{id}/visibility", app.setVisibility)
	prot.HandleFunc("POST /shares/{id}/owners", app.addOwner)
	prot.HandleFunc("POST /shares/{id}/owners/{ownerID}/delete", app.removeOwner)
	prot.HandleFunc("POST /shares/{id}/files/{fileID}/delete", app.deleteFile)
	prot.HandleFunc("GET /trash", app.trashPage)
	prot.HandleFunc("GET /users", app.usersPage)
	prot.HandleFunc("GET /users/new", app.admin(app.newUserPage))
	prot.HandleFunc("POST /users", app.admin(app.createUser))
	prot.HandleFunc("GET /users/{id}/edit", app.admin(app.editUserPage))
	prot.HandleFunc("POST /users/{id}", app.admin(app.updateUser))
	prot.HandleFunc("POST /users/{id}/delete", app.admin(app.deleteUser))
	mux.Handle("/", app.requireLogin(prot))

	api := http.NewServeMux()
	api.HandleFunc("POST /shares", app.createShare)
	api.HandleFunc("POST /shares/{id}/files", app.addFilesToShare)
	api.HandleFunc("HEAD /shares/{id}/files/{fileID}/stream", app.getUploadOffset)
	api.HandleFunc("PUT /shares/{id}/files/{fileID}/stream", app.streamFile)
	api.HandleFunc("DELETE /shares/{id}/files/{fileID}", app.deleteShareFile)
	mux.Handle("/api/", http.StripPrefix("/api", app.requireLoginJSON(api)))

	return app.logging(mux)
}

type page struct {
	Banner      string
	CurrentUser *user.User
	Content     any
}

func (app *App) writePage(w http.ResponseWriter, r *http.Request, template string, content any) {
	t, ok := app.templates[template]
	if !ok {
		log.Printf("template not found: %s", template)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	pg := page{
		Banner:      r.URL.Query().Get("err"),
		CurrentUser: auth.UserFromContext(r.Context()),
		Content:     content,
	}

	buf := new(bytes.Buffer)
	if err := t.ExecuteTemplate(buf, "layout", &pg); err != nil {
		log.Printf("executing template %s: %v", template, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

func (app *App) writeAPI(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (app *App) apiError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := errorResponse(r, err)
	app.writeAPI(w, status, map[string]any{"status": status, "message": message})
}

func (app *App) pageError(w http.ResponseWriter, r *http.Request, err error) {
	status, message := errorResponse(r, err)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, errorPageHTML, status, message)
}

const errorPageHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>error - eigenshare</title>
    <link rel="stylesheet" href="/static/static.css" />
  </head>
  <body>
    <main>
      <h1>%d</h1>
      <p>%s</p>
      <p><a href="/shares">back to shares</a></p>
    </main>
  </body>
</html>
`

func errorResponse(r *http.Request, err error) (int, string) {
	status := http.StatusInternalServerError
	var statusErr interface{ HTTPStatus() int }
	if errors.As(err, &statusErr) {
		status = statusErr.HTTPStatus()
	}
	if status == http.StatusInternalServerError {
		log.Printf("INTERNAL ERROR: %v (path: %s)", err, r.URL.Path)
		return status, "an unexpected error occurred"
	}
	return status, err.Error()
}
