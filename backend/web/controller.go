package web

import (
	"eigenshare/backend/auth"
	"eigenshare/backend/share"
	"eigenshare/backend/user"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Controller struct {
	authService  *auth.AuthService
	shareService *share.ShareService
	pages        map[string]*template.Template
	static       http.Handler
}

func NewController(authSvc *auth.AuthService, shareSvc *share.ShareService) *Controller {
	funcs := template.FuncMap{
		"shareTotalSize": func(sh *share.Share) int64 {
			var total int64
			for _, f := range sh.Files {
				total += f.ExpectedSize
			}
			return total
		},
		"formatSize": formatSize,
	}

	base := template.Must(template.New("layout").Funcs(funcs).ParseFS(templateFS, "templates/layout.html"))

	pages := make(map[string]*template.Template)
	for _, name := range []string{"login", "shares", "share", "edit_share", "new_share", "users", "user_form"} {
		t := template.Must(base.Clone())
		template.Must(t.ParseFS(templateFS, "templates/"+name+".html"))
		pages[name] = t
	}

	staticSub, _ := fs.Sub(staticFS, "static")
	return &Controller{
		authService:  authSvc,
		shareService: shareSvc,
		pages:        pages,
		static:       http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
	}
}

func (c *Controller) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /static/", c.serveStatic)
	router.HandleFunc("GET /login", c.loginPage)
	router.HandleFunc("POST /login", c.loginSubmit)
	router.HandleFunc("POST /logout", c.logout)

	optional := func(h http.HandlerFunc) http.Handler {
		return auth.OptionalAuth(c.authService, h)
	}
	router.Handle("GET /s/{handle}", optional(http.HandlerFunc(c.sharePage)))
	router.Handle("GET /s/{handle}/download", optional(makeHandler(c.downloadAll)))
	router.Handle("GET /s/{handle}/files/{fileID}/download", optional(makeHandler(c.downloadFile)))
}

func (c *Controller) RegisterProtectedHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /", c.index)
	router.HandleFunc("GET /shares", c.sharesPage)
	router.HandleFunc("GET /shares/new", c.newSharePage)
	router.HandleFunc("POST /shares", makeHandler(c.createShare))
	router.HandleFunc("GET /shares/{handle}/edit", c.editSharePage)
	router.HandleFunc("POST /shares/{handle}/delete", c.deleteShare)
	router.HandleFunc("POST /shares/{handle}/files/{fileID}/delete", c.deleteFile)
	router.HandleFunc("GET /shares/{handle}/progress", makeHandler(c.getProgress))
	router.HandleFunc("PUT /shares/{handle}/files/{fileID}/stream", makeHandler(c.streamFile))

	router.HandleFunc("GET /users", c.usersPage)
	admin := auth.RequireAdmin
	router.Handle("GET /users/new", admin(http.HandlerFunc(c.newUserPage)))
	router.Handle("POST /users", admin(http.HandlerFunc(c.createUser)))
	router.Handle("GET /users/{id}/edit", admin(http.HandlerFunc(c.editUserPage)))
	router.Handle("POST /users/{id}", admin(http.HandlerFunc(c.updateUser)))
	router.Handle("POST /users/{id}/delete", admin(http.HandlerFunc(c.deleteUser)))
}

func (c *Controller) render(w http.ResponseWriter, page string, data any) {
	t := c.pages[page]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func page(r *http.Request, title string) Page {
	return Page{Title: title, CurrentUser: auth.UserFromContext(r.Context())}
}

func (c *Controller) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (c *Controller) serveStatic(w http.ResponseWriter, r *http.Request) {
	c.static.ServeHTTP(w, r)
}

func (c *Controller) loginPage(w http.ResponseWriter, r *http.Request) {
	c.render(w, "login", page(r, "login"))
}

func (c *Controller) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		c.render(w, "login", Page{Title: "login", Error: err.Error()})
		return
	}
	cred := auth.Credentials{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := c.authService.Login(cred)
	if err != nil {
		c.render(w, "login", Page{Title: "login", Error: err.Error()})
		return
	}
	auth.SetSessionCookie(w, session.Token, 60*24*7)
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (c *Controller) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = c.authService.Logout(cookie.Value)
	}
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (c *Controller) usersPage(w http.ResponseWriter, r *http.Request) {
	p := page(r, "users")
	users, err := c.authService.GetAllUsers()
	if err != nil {
		p.Error = err.Error()
		c.render(w, "users", UsersView{Page: p})
		return
	}
	c.render(w, "users", UsersView{Page: p, Users: users})
}

func (c *Controller) newUserPage(w http.ResponseWriter, r *http.Request) {
	c.render(w, "user_form", UserFormView{Page: page(r, "new user")})
}

func (c *Controller) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		p := page(r, "new user")
		p.Error = err.Error()
		c.render(w, "user_form", UserFormView{Page: p})
		return
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.authService.CreateUser(req); err != nil {
		p := page(r, "new user")
		p.Error = err.Error()
		c.render(w, "user_form", UserFormView{Page: p})
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (c *Controller) editUserPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := c.authService.GetUser(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	c.render(w, "user_form", UserFormView{Page: page(r, "edit user"), User: u})
}

func (c *Controller) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.authService.UpdateUser(id, auth.UserFromContext(r.Context()).ID, req); err != nil {
		u, _ := c.authService.GetUser(id)
		p := page(r, "edit user")
		p.Error = err.Error()
		c.render(w, "user_form", UserFormView{Page: p, User: u})
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (c *Controller) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	_ = c.authService.DeleteUser(id, actor.ID)
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (c *Controller) sharesPage(w http.ResponseWriter, r *http.Request) {
	p := page(r, "shares")
	shares, err := c.shareService.GetShares()
	if err != nil {
		p.Error = err.Error()
		c.render(w, "shares", SharesView{Page: p})
		return
	}
	c.render(w, "shares", SharesView{Page: p, Shares: shares})
}

func (c *Controller) newSharePage(w http.ResponseWriter, r *http.Request) {
	c.render(w, "new_share", page(r, "new share"))
}

func (c *Controller) sharePage(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	c.render(w, "share", SharePublicView{Page: page(r, sh.Handle), Share: sh})
}

func (c *Controller) editSharePage(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	bytesWritten, _ := c.shareService.GetShareProgress(sh.ID)

	var total int64
	var numFiles int
	for _, f := range sh.Files {
		total += f.ExpectedSize
		numFiles += 1
	}
	c.render(w, "edit_share", ShareView{
		Page:         page(r, sh.Handle),
		Share:        sh,
		numFiles:     numFiles,
		BytesWritten: bytesWritten,
		Complete:     bytesWritten == total && total > 0,
	})
}

func (c *Controller) deleteShare(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = c.shareService.Delete(sh.ID)
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (c *Controller) deleteFile(w http.ResponseWriter, r *http.Request) {
	handle := r.PathValue("handle")
	fileID := r.PathValue("fileID")
	_ = c.shareService.DeleteFile(handle, fileID)
	http.Redirect(w, r, "/shares/"+handle+"/edit", http.StatusFound)
}

func (c *Controller) createShare(w http.ResponseWriter, r *http.Request) error {
	var req share.ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}

	sh, err := c.shareService.RequestShare(req)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusCreated, sh)
}

func (c *Controller) streamFile(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	fileID := r.PathValue("fileID")

	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}

	offset, err := strconv.ParseInt(r.Header.Get("X-Write-Offset"), 10, 64)
	if err != nil {
		return NewError(http.StatusBadRequest, "invalid X-Write-Offset header")
	}

	if err := c.shareService.StreamFile(sh.ID, fileID, offset, r.Body); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) getProgress(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}

	bytesWritten, err := c.shareService.GetShareProgress(sh.ID)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusOK, map[string]int64{"bytesWritten": bytesWritten})
}

func (c *Controller) downloadAll(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="share.zip"`)

	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}

	return c.shareService.DownloadAll(sh.ID, w)
}

func (c *Controller) downloadFile(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	fileID := r.PathValue("fileID")

	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}

	filePath, err := c.shareService.DownloadFile(sh.ID, fileID)
	if err != nil {
		return err
	}

	http.ServeFile(w, r, filePath)
	return nil
}
