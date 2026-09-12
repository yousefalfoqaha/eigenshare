package web

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Controller struct {
	authService  *auth.AuthService
	userService  *user.UserService
	shareService *share.ShareService
	pages        map[string]*template.Template
	static       http.Handler
}

func NewController(authSvc *auth.AuthService, userSvc *user.UserService, shareSvc *share.ShareService) *Controller {
	funcs := template.FuncMap{
		"shareTotalSize":  shareTotalSize,
		"formatSize":      formatSize,
		"shareTotalFiles": shareTotalFiles,
		"expiresIn":       expiresIn,
		"timeAgo":         timeAgo,
	}

	base := template.Must(template.New("layout").Funcs(funcs).ParseFS(templateFS, "templates/layout.html"))

	pages := make(map[string]*template.Template)

	names, err := getTemplateNames()
	if err != nil {
		log.Fatalf("failed to read embedded templates: %v", err)
	}

	for _, name := range names {
		t := template.Must(base.Clone())
		template.Must(t.ParseFS(templateFS, "templates/"+name, "templates/file_picker.html"))
		pages[name] = t
	}

	staticSub, _ := fs.Sub(staticFS, "static")
	return &Controller{
		authService:  authSvc,
		shareService: shareSvc,
		userService:  userSvc,
		pages:        pages,
		static:       http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))),
	}
}

func (c *Controller) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /static/", c.serveStatic)
	router.HandleFunc("GET /login", c.loginPage)
	router.HandleFunc("POST /login", c.loginSubmit)
	router.HandleFunc("POST /logout", c.makeHTMLHandler(c.logout))

	optional := func(h http.HandlerFunc) http.Handler {
		return OptionalAuth(c.authService, h)
	}
	router.Handle("GET /s/{handle}", optional(c.makeHTMLHandler(c.sharePage)))
	router.Handle("GET /s/{handle}/download", optional(makeAPIHandler(c.downloadAll)))
	router.Handle("GET /s/{handle}/files/{fileID}/download", optional(makeAPIHandler(c.downloadFile)))
}

func (c *Controller) RegisterProtectedHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /", c.index)
	router.HandleFunc("GET /shares", c.sharesPage)
	router.HandleFunc("GET /shares/new", c.newSharePage)
	router.HandleFunc("POST /shares", makeAPIHandler(c.createShare))
	router.HandleFunc("POST /shares/delete", c.makeHTMLHandler(c.deleteShares))
	router.HandleFunc("GET /shares/{handle}/edit", c.makeHTMLHandler(c.editSharePage))
	router.HandleFunc("POST /shares/{handle}/trash", c.makeHTMLHandler(c.trashShare))
	router.HandleFunc("POST /shares/{handle}/restore", c.makeHTMLHandler(c.restoreShare))
	router.HandleFunc("POST /shares/{handle}/files", makeAPIHandler(c.addFilesToShare))
	router.HandleFunc("POST /shares/{handle}/files/{fileID}/delete", c.makeHTMLHandler(c.deleteFile))
	router.HandleFunc("PUT /shares/{handle}/files/{fileID}/stream", makeAPIHandler(c.streamFile))
	router.HandleFunc("GET /trash", c.trashPage)

	router.HandleFunc("GET /users", c.usersPage)
	admin := RequireAdmin
	router.Handle("GET /users/new", admin(http.HandlerFunc(c.newUserPage)))
	router.Handle("POST /users", admin(http.HandlerFunc(c.createUser)))
	router.Handle("GET /users/{id}/edit", admin(c.makeHTMLHandler(c.editUserPage)))
	router.Handle("POST /users/{id}", admin(http.HandlerFunc(c.updateUser)))
	router.Handle("POST /users/{id}/delete", admin(c.makeHTMLHandler(c.deleteUser)))
}

func (c *Controller) render(w http.ResponseWriter, page string, data any) {
	c.renderStatus(w, http.StatusOK, page, data)
}

func (c *Controller) renderStatus(w http.ResponseWriter, status int, page string, data any) {
	t := c.pages[page]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *Controller) makeHTMLHandler(h func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			httpErr := MapError(err)
			if httpErr.Status == http.StatusInternalServerError {
				log.Printf("INTERNAL ERROR: %v", err)
			}
			p := page(r, "error")
			p.Error = httpErr.Message
			c.renderStatus(w, httpErr.Status, "error.html", p)
		}
	}
}

func (c *Controller) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (c *Controller) serveStatic(w http.ResponseWriter, r *http.Request) {
	c.static.ServeHTTP(w, r)
}

func (c *Controller) loginPage(w http.ResponseWriter, r *http.Request) {
	c.render(w, "login.html", page(r, "login"))
}

func (c *Controller) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		c.render(w, "login.html", Page{Title: "login", Error: err.Error()})
		return
	}
	cred := auth.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := c.authService.Login(cred)
	if err != nil {
		c.render(w, "login.html", Page{Title: "login", Error: err.Error()})
		return
	}
	SetSessionCookie(w, session.Token, 60*24*7)
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (c *Controller) logout(w http.ResponseWriter, r *http.Request) error {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if err := c.authService.Logout(cookie.Value); err != nil {
			return err
		}
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
	return nil
}

func (c *Controller) usersPage(w http.ResponseWriter, r *http.Request) {
	p := page(r, "users")
	users, err := c.userService.GetAllUsers()
	if err != nil {
		p.Error = err.Error()
		c.render(w, "users.html", UsersView{Page: p})
		return
	}
	c.render(w, "users.html", UsersView{Page: p, Users: users})
}

func (c *Controller) newUserPage(w http.ResponseWriter, r *http.Request) {
	c.render(w, "user_form.html", UserFormView{Page: page(r, "new user")})
}

func (c *Controller) createUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		p := page(r, "new user")
		p.Error = err.Error()
		c.render(w, "user_form.html", UserFormView{Page: p})
		return
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userService.CreateUser(req); err != nil {
		p := page(r, "new user")
		p.Error = err.Error()
		c.render(w, "user_form.html", UserFormView{Page: p})
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (c *Controller) editUserPage(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	u, err := c.userService.GetUser(id)
	if err != nil {
		return err
	}
	c.render(w, "user_form.html", UserFormView{Page: page(r, "edit user"), User: u})
	return nil
}

func (c *Controller) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		p := page(r, "edit user")
		p.Error = err.Error()
		c.render(w, "user_form.html", UserFormView{Page: p})
		return
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userService.UpdateUser(id, auth.UserFromContext(r.Context()).ID, req); err != nil {
		u, getErr := c.userService.GetUser(id)
		p := page(r, "edit user")
		p.Error = err.Error()
		if getErr != nil {
			p.Error = getErr.Error()
		}
		c.render(w, "user_form.html", UserFormView{Page: p, User: u})
		return
	}
	http.Redirect(w, r, "/users", http.StatusFound)
}

func (c *Controller) deleteUser(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	if err := c.userService.DeleteUser(id, actor.ID); err != nil {
		return err
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) sharesPage(w http.ResponseWriter, r *http.Request) {
	p := page(r, "shares")
	shares, err := c.shareService.GetShares()
	if err != nil {
		p.Error = err.Error()
		c.render(w, "shares.html", SharesView{Page: p})
		return
	}
	progress := make(map[string]int64, len(shares))
	for _, s := range shares {
		written, err := c.shareService.GetShareProgress(s.ID)
		if err != nil {
			p.Error = err.Error()
			c.render(w, "shares.html", SharesView{Page: p})
			return
		}
		var total int64
		for _, w := range written {
			total += w
		}
		progress[s.ID] = total
	}
	c.render(w, "shares.html", SharesView{Page: p, Shares: shares, Progress: progress})
}

func (c *Controller) trashPage(w http.ResponseWriter, r *http.Request) {
	p := page(r, "trash")
	shares, err := c.shareService.GetTrashedShares()
	if err != nil {
		p.Error = err.Error()
		c.render(w, "trash.html", SharesView{Page: p})
		return
	}
	c.render(w, "trash.html", SharesView{Page: p, Shares: shares})
}

func (c *Controller) newSharePage(w http.ResponseWriter, r *http.Request) {
	c.render(w, "new_share.html", page(r, "new share"))
}

func (c *Controller) sharePage(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}

	c.render(w, "share.html", SharePublicView{Page: page(r, sh.Handle), Share: sh})
	return nil
}

func (c *Controller) editSharePage(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}
	progress, err := c.shareService.GetShareProgress(sh.ID)
	if err != nil {
		return err
	}

	c.render(w, "edit_share.html", ShareView{
		Page:     page(r, sh.Handle),
		Share:    sh,
		Progress: progress,
	})
	return nil
}

func (c *Controller) trashShare(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}
	if err := c.shareService.TrashShare(sh.ID); err != nil {
		return err
	}
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) restoreShare(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}
	if err := c.shareService.RecoverShare(sh.ID); err != nil {
		return err
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
	return nil
}

func (c *Controller) deleteShares(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := c.shareService.Delete(r.PostForm["shareID"]...); err != nil {
		return err
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
	return nil
}

func (c *Controller) addFilesToShare(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")

	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}

	newFiles, err := c.shareService.UploadFiles(handle, req.Files)
	if err != nil {
		return err
	}

	return writeAPIResponse(w, http.StatusOK, newFiles)
}

func (c *Controller) deleteFile(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	fileID := r.PathValue("fileID")
	if err := c.shareService.DeleteFile(handle, fileID); err != nil {
		return err
	}
	http.Redirect(w, r, "/shares/"+handle+"/edit", http.StatusFound)
	return nil
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

	return writeAPIResponse(w, http.StatusCreated, sh)
}

func (c *Controller) streamFile(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	fileID := r.PathValue("fileID")

	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}

	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return NewHTTPError(http.StatusBadRequest, "invalid Upload-Offset header")
	}

	bytesWritten, err := c.shareService.StreamFile(sh.ID, fileID, offset, r.Body)
	if err != nil {
		if offsetError, ok := errors.AsType[*share.ChunkOffsetConflictError](err); ok {
			w.Header().Set("Upload-Offset", strconv.FormatInt(offsetError.CurrentOffset, 10))
			return NewHTTPError(http.StatusConflict, err.Error())
		}
		return err
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
	return nil
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

func getTemplateNames() ([]string, error) {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, err
	}

	var templateFiles []string
	for _, file := range entries {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if name == "layout.html" || name == "file_picker.html" {
			continue
		}

		templateFiles = append(templateFiles, name)
	}

	return templateFiles, nil
}
