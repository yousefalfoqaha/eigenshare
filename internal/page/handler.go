package page

import (
	"mime"
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/middleware"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func (c *Controller) Index(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) LoginPage(w http.ResponseWriter, r *http.Request) error {
	return c.render(w, r, "login.html", nil, nil)
}

func (c *Controller) LoginSubmit(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return c.render(w, r, "login.html", nil, err)
	}
	cred := auth.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := c.Auth.Login(cred)
	if err != nil {
		return c.render(w, r, "login.html", nil, err)
	}
	middleware.SetSessionCookie(w, session.Token, 60*24*7)
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) Logout(w http.ResponseWriter, r *http.Request) error {
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		_ = c.Auth.Logout(cookie.Value)
	}
	middleware.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
	return nil
}

func (c *Controller) SharePage(w http.ResponseWriter, r *http.Request) error {
	actor := auth.UserFromContext(r.Context())

	sh, err := c.Share.GetShareByHandle(r.PathValue("handle"), actor)
	if err != nil {
		return err
	}
	upload, err := c.Share.GetUploadState(sh, actor)
	if err != nil {
		return err
	}

	return c.render(w, r, "share.html", &struct {
		Share  *share.Share
		Upload *share.UploadState
	}{sh, upload}, nil)
}

// --- users ---

func (c *Controller) UsersPage(w http.ResponseWriter, r *http.Request) error {
	users, err := c.User.GetAllUsers()
	if err != nil {
		return err
	}
	return c.render(w, r, "users.html", users, nil)
}

func (c *Controller) NewUserPage(w http.ResponseWriter, r *http.Request) error {
	return c.render(w, r, "user_form.html", nil, nil)
}

func (c *Controller) CreateUser(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return c.render(w, r, "user_form.html", nil, err)
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.User.CreateUser(req); err != nil {
		return c.render(w, r, "user_form.html", nil, err)
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) EditUserPage(w http.ResponseWriter, r *http.Request) error {
	u, err := c.User.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	return c.render(w, r, "user_form.html", u, nil)
}

func (c *Controller) UpdateUser(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		return c.render(w, r, "user_form.html", nil, err)
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.User.UpdateUser(id, auth.UserFromContext(r.Context()).ID, req); err != nil {
		u, _ := c.User.GetUser(id)
		return c.render(w, r, "user_form.html", u, err)
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) DeleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := c.User.DeleteUser(r.PathValue("id"), auth.UserFromContext(r.Context()).ID); err != nil {
		users, listErr := c.User.GetAllUsers()
		if listErr != nil {
			return listErr
		}
		return c.render(w, r, "users.html", users, err)
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

// --- shares ---

type sharesData struct {
	Shares     []*share.Share
	Uploads    map[string]*share.UploadState
	OwnerNames map[string][]string
}

func (c *Controller) buildSharesData(actor *user.User) (*sharesData, error) {
	shares, err := c.Share.GetShares(actor)
	if err != nil {
		return nil, err
	}

	uploads := make(map[string]*share.UploadState, len(shares))
	for _, s := range shares {
		u, err := c.Share.GetUploadState(s, actor)
		if err != nil {
			return nil, err
		}
		uploads[s.ID] = u
	}

	users, _ := c.User.GetAllUsers()
	namesByID := make(map[string]string, len(users))
	for _, u := range users {
		namesByID[u.ID] = u.Username
	}

	ownerNames := make(map[string][]string, len(shares))
	for _, s := range shares {
		names := make([]string, 0, len(s.Owners))
		for _, id := range s.Owners {
			if name, ok := namesByID[id]; ok {
				names = append(names, name)
			}
		}
		ownerNames[s.ID] = names
	}

	return &sharesData{shares, uploads, ownerNames}, nil
}

func (c *Controller) SharesPage(w http.ResponseWriter, r *http.Request) error {
	data, err := c.buildSharesData(auth.UserFromContext(r.Context()))
	if err != nil {
		return err
	}
	return c.render(w, r, "shares.html", data, nil)
}

func (c *Controller) NewSharePage(w http.ResponseWriter, r *http.Request) error {
	return c.render(w, r, "new_share.html", nil, nil)
}

type trashData struct {
	Shares  []*share.Share
	Uploads map[string]*share.UploadState
}

func (c *Controller) buildTrashData(actor *user.User) (*trashData, error) {
	shares, err := c.Share.GetTrashedShares(actor)
	if err != nil {
		return nil, err
	}

	uploads := make(map[string]*share.UploadState, len(shares))
	for _, s := range shares {
		u, err := c.Share.GetUploadState(s, actor)
		if err != nil {
			return nil, err
		}
		uploads[s.ID] = u
	}

	return &trashData{shares, uploads}, nil
}

func (c *Controller) TrashPage(w http.ResponseWriter, r *http.Request) error {
	data, err := c.buildTrashData(auth.UserFromContext(r.Context()))
	if err != nil {
		return err
	}
	return c.render(w, r, "trash.html", data, nil)
}

type editShareData struct {
	Share      *share.Share
	Upload     *share.UploadState
	Users      []*user.User
	Tab        string
	OwnerNames []string
}

func (c *Controller) buildEditShareData(actor *user.User, id, tab string) (*editShareData, error) {
	sh, err := c.Share.GetShare(id, actor)
	if err != nil {
		return nil, err
	}

	upload, err := c.Share.GetUploadState(sh, actor)
	if err != nil {
		return nil, err
	}

	users, _ := c.User.GetAllUsers()

	if tab != "owners" {
		tab = "files"
	}

	ownerNames := make([]string, 0, len(sh.Owners))
	for _, u := range users {
		if sh.IsOwner(u) {
			ownerNames = append(ownerNames, u.Username)
		}
	}

	return &editShareData{sh, upload, users, tab, ownerNames}, nil
}

func (c *Controller) EditSharePage(w http.ResponseWriter, r *http.Request) error {
	data, err := c.buildEditShareData(auth.UserFromContext(r.Context()), r.PathValue("id"), r.URL.Query().Get("tab"))
	if err != nil {
		return err
	}
	return c.render(w, r, "edit_share.html", data, nil)
}

func (c *Controller) TrashShare(w http.ResponseWriter, r *http.Request) error {
	actor := auth.UserFromContext(r.Context())
	if err := c.Share.TrashShare(r.PathValue("id"), actor); err != nil {
		data, dataErr := c.buildSharesData(actor)
		if dataErr != nil {
			return dataErr
		}
		return c.render(w, r, "shares.html", data, err)
	}
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) RestoreShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	toEdit := r.PostFormValue("redirect") == "edit"

	if err := c.Share.RecoverShare(id, actor); err != nil {
		if toEdit {
			data, dataErr := c.buildEditShareData(actor, id, "files")
			if dataErr != nil {
				return dataErr
			}
			return c.render(w, r, "edit_share.html", data, err)
		}
		data, dataErr := c.buildTrashData(actor)
		if dataErr != nil {
			return dataErr
		}
		return c.render(w, r, "trash.html", data, err)
	}

	if toEdit {
		http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
	} else {
		http.Redirect(w, r, "/trash", http.StatusFound)
	}
	return nil
}

func (c *Controller) DeleteShares(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return c.renderTrash(w, r, err)
	}
	if err := c.Share.Delete(auth.UserFromContext(r.Context()), r.PostForm["shareID"]...); err != nil {
		return c.renderTrash(w, r, err)
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
	return nil
}

func (c *Controller) renderTrash(w http.ResponseWriter, r *http.Request, domainErr error) error {
	data, err := c.buildTrashData(auth.UserFromContext(r.Context()))
	if err != nil {
		return err
	}
	return c.render(w, r, "trash.html", data, domainErr)
}

func (c *Controller) DeleteFile(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	if err := c.Share.DeleteFile(id, r.PathValue("fileID"), actor); err != nil {
		data, dataErr := c.buildEditShareData(actor, id, "files")
		if dataErr != nil {
			return dataErr
		}
		return c.render(w, r, "edit_share.html", data, err)
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
	return nil
}

func (c *Controller) AddOwner(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		return c.renderEditOwners(w, r, actor, id, err)
	}
	if err := c.Share.AddOwner(id, actor, r.PostForm.Get("userID")); err != nil {
		return c.renderEditOwners(w, r, actor, id, err)
	}
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusFound)
	return nil
}

func (c *Controller) RemoveOwner(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	if err := c.Share.RemoveOwner(id, actor, r.PathValue("ownerID")); err != nil {
		return c.renderEditOwners(w, r, actor, id, err)
	}
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusFound)
	return nil
}

func (c *Controller) renderEditOwners(w http.ResponseWriter, r *http.Request, actor *user.User, id string, domainErr error) error {
	data, err := c.buildEditShareData(actor, id, "owners")
	if err != nil {
		return err
	}
	return c.render(w, r, "edit_share.html", data, domainErr)
}

func (c *Controller) SetVisibility(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := auth.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		return c.renderEditFiles(w, r, actor, id, err)
	}
	if err := c.Share.SetVisibility(id, actor, share.Visibility(r.PostForm.Get("visibility"))); err != nil {
		return c.renderEditFiles(w, r, actor, id, err)
	}
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusFound)
	return nil
}

func (c *Controller) renderEditFiles(w http.ResponseWriter, r *http.Request, actor *user.User, id string, domainErr error) error {
	data, err := c.buildEditShareData(actor, id, "files")
	if err != nil {
		return err
	}
	return c.render(w, r, "edit_share.html", data, domainErr)
}

// contentDisposition builds an "attachment" header value. file names (and
// share handles) end up in here, so use FormatMediaType: it quotes/escapes
// per RFC 2045 and percent-encodes anything unsafe per RFC 2231
func contentDisposition(filename string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}

func (c *Controller) DownloadAll(w http.ResponseWriter, r *http.Request) error {
	sh, err := c.Share.GetShareByHandle(r.PathValue("handle"), auth.UserFromContext(r.Context()))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(sh.Handle+".zip"))
	return c.Share.DownloadAll(sh.ID, w)
}

func (c *Controller) DownloadFile(w http.ResponseWriter, r *http.Request) error {
	sh, err := c.Share.GetShareByHandle(r.PathValue("handle"), auth.UserFromContext(r.Context()))
	if err != nil {
		return err
	}

	file, filePath, err := c.Share.DownloadFile(sh.ID, r.PathValue("fileID"))
	if err != nil {
		return err
	}

	// the on-disk path is a bare file id, so name the download after the
	// original file; without this the browser saves it as "download"
	w.Header().Set("Content-Disposition", contentDisposition(file.Name))
	http.ServeFile(w, r, filePath)
	return nil
}
