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
	router.HandleFunc("GET /login", makeHTMLHandler(c, "login.html", c.loginPage))
	router.HandleFunc("POST /login", makeHTMLHandler(c, "login.html", c.loginSubmit))
	router.HandleFunc("POST /logout", makeHTMLHandler(c, "login.html", c.logout))

	optional := func(h http.Handler) http.Handler {
		return OptionalAuth(c.authService, h)
	}
	router.Handle("GET /s/{handle}", optional(makeHTMLHandler(c, "share.html", c.sharePage)))
	router.Handle("GET /s/{handle}/download", optional(makeAPIHandler(c.downloadAll)))
	router.Handle("GET /s/{handle}/files/{fileID}/download", optional(makeAPIHandler(c.downloadFile)))
}

func (c *Controller) RegisterProtectedHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /", c.index)
	router.HandleFunc("GET /shares", makeHTMLHandler(c, "shares.html", c.sharesPage))
	router.HandleFunc("GET /shares/new", makeHTMLHandler(c, "new_share.html", c.newSharePage))
	router.HandleFunc("POST /shares", makeAPIHandler(c.createShare))
	router.HandleFunc("POST /shares/delete", makeHTMLHandler(c, "trash.html", c.deleteShares))
	router.HandleFunc("GET /shares/{id}/edit", makeHTMLHandler(c, "edit_share.html", c.editSharePage))
	router.HandleFunc("POST /shares/{id}/trash", makeHTMLHandler(c, "shares.html", c.trashShare))
	router.HandleFunc("POST /shares/{id}/restore", makeHTMLHandler(c, "trash.html", c.restoreShare))
	router.HandleFunc("POST /shares/{id}/files", makeAPIHandler(c.addFilesToShare))
	router.HandleFunc("POST /shares/{id}/files/{fileID}/delete", makeHTMLHandler(c, "edit_share.html", c.deleteFile))
	router.HandleFunc("PUT /shares/{id}/files/{fileID}/stream", makeAPIHandler(c.streamFile))
	router.HandleFunc("GET /trash", makeHTMLHandler(c, "trash.html", c.trashPage))

	router.HandleFunc("GET /users", makeHTMLHandler(c, "users.html", c.usersPage))
	admin := RequireAdmin
	router.Handle("GET /users/new", admin(makeHTMLHandler(c, "user_form.html", c.newUserPage)))
	router.Handle("POST /users", admin(makeHTMLHandler(c, "user_form.html", c.createUser)))
	router.Handle("GET /users/{id}/edit", admin(makeHTMLHandler(c, "user_form.html", c.editUserPage)))
	router.Handle("POST /users/{id}", admin(makeHTMLHandler(c, "user_form.html", c.updateUser)))
	router.Handle("POST /users/{id}/delete", admin(makeHTMLHandler(c, "users.html", c.deleteUser)))
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

// makeHTMLHandler wraps a page handler in the writer pattern: the handler is
// handed the page and an empty view, and writes into both (p.Title, the
// view's fields). It returns bare errors, and writes to the response only to
// redirect, which the wrapper detects. After the handler runs, the wrapper
// composes the page (p.Content = view), writes the error banner if the
// handler failed, and renders the registered template at the proper status.
func makeHTMLHandler[V any](c *Controller, tmpl string, h func(w http.ResponseWriter, p *Page, v *V, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := page(r, "")
		v := new(V)
		rw := &WrappedWriter{ResponseWriter: w, status: http.StatusOK}
		err := h(rw, p, v, r)
		p.Content = v
		if rw.written {
			if err != nil {
				log.Printf("INTERNAL ERROR: handler wrote response and returned error: %v", err)
			}
			return
		}
		if err != nil {
			httpErr := MapError(err)
			if httpErr.Status == http.StatusInternalServerError {
				log.Printf("INTERNAL ERROR: %v", err)
			}
			p.Error = httpErr.Message
			c.renderStatus(rw, httpErr.Status, tmpl, p)
			return
		}
		c.render(rw, tmpl, p)
	}
}

func (c *Controller) index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/shares", http.StatusFound)
}

func (c *Controller) serveStatic(w http.ResponseWriter, r *http.Request) {
	c.static.ServeHTTP(w, r)
}

func (c *Controller) loginPage(w http.ResponseWriter, p *Page, _ *struct{}, r *http.Request) error {
	p.Title = "login"
	return nil
}

func (c *Controller) loginSubmit(w http.ResponseWriter, p *Page, _ *struct{}, r *http.Request) error {
	p.Title = "login"
	if err := r.ParseForm(); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error())
	}
	cred := auth.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := c.authService.Login(cred)
	if err != nil {
		return err
	}
	SetSessionCookie(w, session.Token, 60*24*7)
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) logout(w http.ResponseWriter, p *Page, _ *struct{}, r *http.Request) error {
	p.Title = "login"
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if err := c.authService.Logout(cookie.Value); err != nil {
			return err
		}
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
	return nil
}

func (c *Controller) usersPage(w http.ResponseWriter, p *Page, v *UsersContent, r *http.Request) error {
	p.Title = "users"
	users, err := c.userService.GetAllUsers()
	if err != nil {
		return err
	}
	v.Users = users
	return nil
}

func (c *Controller) newUserPage(w http.ResponseWriter, p *Page, _ *UserFormContent, r *http.Request) error {
	p.Title = "new user"
	return nil
}

func (c *Controller) createUser(w http.ResponseWriter, p *Page, _ *UserFormContent, r *http.Request) error {
	p.Title = "new user"
	if err := r.ParseForm(); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userService.CreateUser(req); err != nil {
		return err
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) editUserPage(w http.ResponseWriter, p *Page, v *UserFormContent, r *http.Request) error {
	p.Title = "edit user"
	u, err := c.userService.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	v.User = u
	return nil
}

func (c *Controller) updateUser(w http.ResponseWriter, p *Page, v *UserFormContent, r *http.Request) error {
	p.Title = "edit user"
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userService.UpdateUser(id, auth.UserFromContext(r.Context()).ID, req); err != nil {
		if u, getErr := c.userService.GetUser(id); getErr == nil {
			v.User = u
		}
		return err
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) deleteUser(w http.ResponseWriter, p *Page, v *UsersContent, r *http.Request) error {
	p.Title = "users"
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	if err := c.userService.DeleteUser(id, actor.ID); err != nil {
		if users, getErr := c.userService.GetAllUsers(); getErr == nil {
			v.Users = users
		}
		return err
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) sharesPage(w http.ResponseWriter, p *Page, v *SharesContent, r *http.Request) error {
	p.Title = "shares"
	shares, err := c.shareService.GetShares()
	if err != nil {
		return err
	}
	v.Shares = shares
	progress := make(map[string]int64, len(shares))
	for _, s := range shares {
		written, err := c.shareService.GetShareProgress(s.ID)
		if err != nil {
			return err
		}
		var total int64
		for _, n := range written {
			total += n
		}
		progress[s.ID] = total
	}
	v.Progress = progress
	return nil
}

func (c *Controller) trashPage(w http.ResponseWriter, p *Page, v *SharesContent, r *http.Request) error {
	p.Title = "trash"
	shares, err := c.shareService.GetTrashedShares()
	if err != nil {
		return err
	}
	v.Shares = shares
	return nil
}

func (c *Controller) newSharePage(w http.ResponseWriter, p *Page, _ *struct{}, r *http.Request) error {
	p.Title = "new share"
	return nil
}

func (c *Controller) sharePage(w http.ResponseWriter, p *Page, v *ShareContent, r *http.Request) error {
	handle := r.PathValue("handle")
	p.Title = handle
	sh, err := c.shareService.GetShareByHandle(handle)
	if err != nil {
		return err
	}
	v.Share = sh
	return nil
}

func (c *Controller) editSharePage(w http.ResponseWriter, p *Page, v *EditShareContent, r *http.Request) error {
	sh, err := c.shareService.GetShare(r.PathValue("id"))
	if err != nil {
		return err
	}
	p.Title = sh.Handle
	v.Share = sh
	progress, err := c.shareService.GetShareProgress(sh.ID)
	if err != nil {
		return err
	}
	v.Progress = progress
	return nil
}

func (c *Controller) trashShare(w http.ResponseWriter, p *Page, _ *SharesContent, r *http.Request) error {
	p.Title = "shares"
	if err := c.shareService.TrashShare(r.PathValue("id")); err != nil {
		return err
	}
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) restoreShare(w http.ResponseWriter, p *Page, _ *SharesContent, r *http.Request) error {
	p.Title = "trash"
	if err := c.shareService.RecoverShare(r.PathValue("id")); err != nil {
		return err
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
	return nil
}

func (c *Controller) deleteShares(w http.ResponseWriter, p *Page, _ *SharesContent, r *http.Request) error {
	p.Title = "trash"
	if err := r.ParseForm(); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := c.shareService.Delete(r.PostForm["shareID"]...); err != nil {
		return err
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
	return nil
}

func (c *Controller) deleteFile(w http.ResponseWriter, p *Page, v *EditShareContent, r *http.Request) error {
	id := r.PathValue("id")
	sh, err := c.shareService.GetShare(id)
	if err != nil {
		return err
	}
	p.Title = sh.Handle
	if err := c.shareService.DeleteFile(id, r.PathValue("fileID")); err != nil {
		v.Share = sh
		return err
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
	return nil
}

func (c *Controller) addFilesToShare(w http.ResponseWriter, r *http.Request) error {
	shareID := r.PathValue("id")

	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}

	newFiles, err := c.shareService.UploadFiles(shareID, req.Files)
	if err != nil {
		return err
	}

	return writeAPIResponse(w, http.StatusOK, newFiles)
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
	shareID := r.PathValue("id")
	fileID := r.PathValue("fileID")

	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return NewHTTPError(http.StatusBadRequest, "invalid Upload-Offset header")
	}

	bytesWritten, err := c.shareService.StreamFile(shareID, fileID, offset, r.Body)
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
