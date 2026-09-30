package page

import (
	"mime"
	"net/http"

	"eigenshare/internal/security"
	"eigenshare/internal/session"
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
	cred := session.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	session, err := c.sessionSvc.Login(cred)
	if err != nil {
		return c.render(w, r, "login.html", nil, err)
	}
	c.sessionCookie.Set(w, session.Token)
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) Logout(w http.ResponseWriter, r *http.Request) error {
	if token, ok := c.sessionCookie.Get(r); ok {
		_ = c.sessionSvc.Logout(token)
	}

	c.sessionCookie.Clear(w)
	http.Redirect(w, r, "/login", http.StatusFound)

	return nil
}

func (c *Controller) SharePage(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())

	sh, err := c.shareSvc.GetShareByHandle(r.PathValue("handle"), actor)
	if err != nil {
		return err
	}

	if sh.RequiresPassword(actor) && !c.shareUnlockCookie.WithID(sh.ID).CheckTag(r, c.shareUnlockSecret, sh.UnlockMessage()) {
		return c.render(w, r, "share_unlock.html", &struct{ Handle string }{sh.Handle}, nil)
	}

	upload, err := c.shareSvc.GetUploadState(sh, actor)
	if err != nil {
		return err
	}

	return c.render(w, r, "share.html", &struct {
		Share  *share.Share
		Upload *share.UploadState
	}{sh, upload}, nil)
}

func (c *Controller) UnlockShare(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	actor := session.UserFromContext(r.Context())

	sh, err := c.shareSvc.GetShareByHandle(handle, actor)
	if err != nil {
		return err
	}

	if !sh.RequiresPassword(actor) {
		http.Redirect(w, r, "/s/"+handle, http.StatusFound)
		return nil
	}

	if err := r.ParseForm(); err != nil {
		return c.render(w, r, "share_unlock.html", &struct{ Handle string }{sh.Handle}, err)
	}

	if !security.VerifyPassword(sh.PasswordHash, r.PostForm.Get("password")) {
		return c.render(w, r, "share_unlock.html", &struct{ Handle string }{sh.Handle}, security.ErrWrongPassword)
	}

	c.shareUnlockCookie.WithID(sh.ID).SetTag(w, c.shareUnlockSecret, sh.UnlockMessage())
	http.Redirect(w, r, "/s/"+handle, http.StatusFound)
	return nil
}

// --- users ---

func (c *Controller) UsersPage(w http.ResponseWriter, r *http.Request) error {
	users, err := c.userSvc.GetAllUsers()
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
	if _, err := c.userSvc.CreateUser(req); err != nil {
		return c.render(w, r, "user_form.html", nil, err)
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) EditUserPage(w http.ResponseWriter, r *http.Request) error {
	u, err := c.userSvc.GetUser(r.PathValue("id"))
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
	if _, err := c.userSvc.UpdateUser(id, session.UserFromContext(r.Context()).ID, req); err != nil {
		u, _ := c.userSvc.GetUser(id)
		return c.render(w, r, "user_form.html", u, err)
	}
	http.Redirect(w, r, "/users", http.StatusFound)
	return nil
}

func (c *Controller) DeleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := c.userSvc.DeleteUser(r.PathValue("id"), session.UserFromContext(r.Context()).ID); err != nil {
		users, listErr := c.userSvc.GetAllUsers()
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
	shares, err := c.shareSvc.GetShares(actor)
	if err != nil {
		return nil, err
	}

	uploads := make(map[string]*share.UploadState, len(shares))
	for _, s := range shares {
		u, err := c.shareSvc.GetUploadState(s, actor)
		if err != nil {
			return nil, err
		}
		uploads[s.ID] = u
	}

	users, _ := c.userSvc.GetAllUsers()
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
	data, err := c.buildSharesData(session.UserFromContext(r.Context()))
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
	shares, err := c.shareSvc.GetTrashedShares(actor)
	if err != nil {
		return nil, err
	}

	uploads := make(map[string]*share.UploadState, len(shares))
	for _, s := range shares {
		u, err := c.shareSvc.GetUploadState(s, actor)
		if err != nil {
			return nil, err
		}
		uploads[s.ID] = u
	}

	return &trashData{shares, uploads}, nil
}

func (c *Controller) TrashPage(w http.ResponseWriter, r *http.Request) error {
	data, err := c.buildTrashData(session.UserFromContext(r.Context()))
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
	sh, err := c.shareSvc.GetShare(id, actor)
	if err != nil {
		return nil, err
	}

	upload, err := c.shareSvc.GetUploadState(sh, actor)
	if err != nil {
		return nil, err
	}

	users, _ := c.userSvc.GetAllUsers()

	switch tab {
	case "files", "owners", "settings":
	default:
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
	data, err := c.buildEditShareData(session.UserFromContext(r.Context()), r.PathValue("id"), r.URL.Query().Get("tab"))
	if err != nil {
		return err
	}
	return c.render(w, r, "edit_share.html", data, nil)
}

func (c *Controller) TrashShare(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())
	if err := c.shareSvc.TrashShare(r.PathValue("id"), actor); err != nil {
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
	actor := session.UserFromContext(r.Context())
	toEdit := r.PostFormValue("redirect") == "edit"

	if err := c.shareSvc.RecoverShare(id, actor); err != nil {
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
	if err := c.shareSvc.Delete(session.UserFromContext(r.Context()), r.PostForm["shareID"]...); err != nil {
		return c.renderTrash(w, r, err)
	}
	http.Redirect(w, r, "/trash", http.StatusFound)
	return nil
}

func (c *Controller) renderTrash(w http.ResponseWriter, r *http.Request, domainErr error) error {
	data, err := c.buildTrashData(session.UserFromContext(r.Context()))
	if err != nil {
		return err
	}
	return c.render(w, r, "trash.html", data, domainErr)
}

func (c *Controller) DeleteFile(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := session.UserFromContext(r.Context())
	if err := c.shareSvc.DeleteFile(id, r.PathValue("fileID"), actor); err != nil {
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
	actor := session.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		return c.renderEditOwners(w, r, actor, id, err)
	}
	if err := c.shareSvc.AddOwner(id, actor, r.PostForm.Get("userID")); err != nil {
		return c.renderEditOwners(w, r, actor, id, err)
	}
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusFound)
	return nil
}

func (c *Controller) RemoveOwner(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := session.UserFromContext(r.Context())
	if err := c.shareSvc.RemoveOwner(id, actor, r.PathValue("ownerID")); err != nil {
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

func (c *Controller) UpdateShareSettings(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := session.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		return c.renderEditSettings(w, r, actor, id, err)
	}
	if err := c.shareSvc.SetPassword(id, actor, r.PostForm.Get("password"), r.PostForm.Get("removePassword") == "on"); err != nil {
		return c.renderEditSettings(w, r, actor, id, err)
	}
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=settings", http.StatusFound)
	return nil
}

func (c *Controller) renderEditSettings(w http.ResponseWriter, r *http.Request, actor *user.User, id string, domainErr error) error {
	data, err := c.buildEditShareData(actor, id, "settings")
	if err != nil {
		return err
	}
	return c.render(w, r, "edit_share.html", data, domainErr)
}

func (c *Controller) SetVisibility(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := session.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		return c.renderEditFiles(w, r, actor, id, err)
	}
	if err := c.shareSvc.SetVisibility(id, actor, share.Visibility(r.PostForm.Get("visibility"))); err != nil {
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

func contentDisposition(filename string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}

func (c *Controller) DownloadAll(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())
	sh, err := c.shareSvc.GetShareByHandle(r.PathValue("handle"), actor)
	if err != nil {
		return err
	}

	if sh.RequiresPassword(actor) && !c.shareUnlockCookie.WithID(sh.ID).CheckTag(r, c.shareUnlockSecret, sh.UnlockMessage()) {
		http.Redirect(w, r, "/s/"+sh.Handle, http.StatusFound)
		return nil
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(sh.Handle+".zip"))
	return c.shareSvc.DownloadAll(sh.ID, w)
}

func (c *Controller) DownloadFile(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())
	sh, err := c.shareSvc.GetShareByHandle(r.PathValue("handle"), actor)
	if err != nil {
		return err
	}

	if sh.RequiresPassword(actor) && !c.shareUnlockCookie.WithID(sh.ID).CheckTag(r, c.shareUnlockSecret, sh.UnlockMessage()) {
		http.Redirect(w, r, "/s/"+sh.Handle, http.StatusFound)
		return nil
	}

	file, filePath, err := c.shareSvc.DownloadFile(sh.ID, r.PathValue("fileID"))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Disposition", contentDisposition(file.Name))
	http.ServeFile(w, r, filePath)
	return nil
}
