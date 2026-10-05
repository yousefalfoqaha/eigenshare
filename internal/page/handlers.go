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
	return c.renderPage(w, r, http.StatusOK, "login.html", nil)
}

func (c *Controller) LoginSubmit(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	cred := session.LoginRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
	}
	sess, err := c.sessionSvc.Login(cred)
	if err != nil {
		return err
	}
	c.sessionCookie.Set(w, sess.Token)
	c.flash.Success(w, "logged in")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) Logout(w http.ResponseWriter, r *http.Request) error {
	if token, ok := c.sessionCookie.Get(r); ok {
		_ = c.sessionSvc.Logout(token)
	}

	c.sessionCookie.Clear(w)
	c.flash.Success(w, "logged out")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	return nil
}

func (c *Controller) SharePage(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())

	sh, err := c.shareSvc.GetShareByHandle(r.PathValue("handle"), actor)
	if err != nil {
		return err
	}

	if sh.RequiresPassword(actor) && !c.shareUnlockCookie.WithID(sh.ID).CheckTag(r, c.shareUnlockSecret, sh.UnlockMessage()) {
		return c.renderPage(w, r, http.StatusOK, "share_unlock.html", &struct{ Handle string }{sh.Handle})
	}

	states, err := c.shareSvc.GetShareUploadState(sh, actor)
	if err != nil {
		return err
	}

	return c.renderPage(w, r, http.StatusOK, "share.html", &struct {
		Share  *share.Share
		Upload map[string]share.FileUploadState
	}{sh, states})
}

func (c *Controller) UnlockShare(w http.ResponseWriter, r *http.Request) error {
	handle := r.PathValue("handle")
	actor := session.UserFromContext(r.Context())

	sh, err := c.shareSvc.GetShareByHandle(handle, actor)
	if err != nil {
		return err
	}

	if !sh.RequiresPassword(actor) {
		http.Redirect(w, r, "/s/"+handle, http.StatusSeeOther)
		return nil
	}

	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}

	if !security.VerifyPassword(sh.PasswordHash, r.PostForm.Get("password")) {
		return security.ErrWrongPassword
	}

	c.shareUnlockCookie.WithID(sh.ID).SetTag(w, c.shareUnlockSecret, sh.UnlockMessage())
	c.flash.Success(w, "share unlocked")
	http.Redirect(w, r, "/s/"+handle, http.StatusSeeOther)
	return nil
}

func (c *Controller) UsersPage(w http.ResponseWriter, r *http.Request) error {
	users, err := c.userSvc.GetAllUsers()
	if err != nil {
		return err
	}
	return c.renderPage(w, r, http.StatusOK, "users.html", users)
}

func (c *Controller) NewUserPage(w http.ResponseWriter, r *http.Request) error {
	return c.renderPage(w, r, http.StatusOK, "user_form.html", nil)
}

func (c *Controller) CreateUser(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	req := user.CreateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userSvc.CreateUser(req); err != nil {
		return err
	}
	c.flash.Success(w, "user created")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) EditUserPage(w http.ResponseWriter, r *http.Request) error {
	u, err := c.userSvc.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	return c.renderPage(w, r, http.StatusOK, "user_form.html", u)
}

func (c *Controller) UpdateUser(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	req := user.UpdateRequest{
		Username: r.PostForm.Get("username"),
		Password: r.PostForm.Get("password"),
		IsAdmin:  r.PostForm.Get("isAdmin") == "on",
	}
	if _, err := c.userSvc.UpdateUser(r.PathValue("id"), session.UserFromContext(r.Context()).ID, req); err != nil {
		return err
	}
	c.flash.Success(w, "user updated")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) DeleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := c.userSvc.DeleteUser(r.PathValue("id"), session.UserFromContext(r.Context()).ID); err != nil {
		return err
	}
	c.flash.Success(w, "user deleted")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) SharesPage(w http.ResponseWriter, r *http.Request) error {
	summaries, err := c.shareSvc.ListSummaries(session.UserFromContext(r.Context()))
	if err != nil {
		return err
	}
	return c.renderPage(w, r, http.StatusOK, "shares.html", summaries)
}

func (c *Controller) NewSharePage(w http.ResponseWriter, r *http.Request) error {
	return c.renderPage(w, r, http.StatusOK, "new_share.html", nil)
}

func (c *Controller) TrashPage(w http.ResponseWriter, r *http.Request) error {
	summaries, err := c.shareSvc.ListTrashedSummaries(session.UserFromContext(r.Context()))
	if err != nil {
		return err
	}
	return c.renderPage(w, r, http.StatusOK, "trash.html", summaries)
}

func (c *Controller) EditSharePage(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())

	summary, err := c.shareSvc.GetSummary(r.PathValue("id"), actor)
	if err != nil {
		return err
	}

	users, _ := c.userSvc.GetAllUsers()

	tab := r.URL.Query().Get("tab")
	switch tab {
	case "files", "owners", "settings":
	default:
		tab = "files"
	}

	return c.renderPage(w, r, http.StatusOK, "edit_share.html", &struct {
		Share      *share.Share
		Upload     map[string]share.FileUploadState
		Users      []*user.User
		Tab        string
		OwnerNames []string
	}{summary.Share, summary.Upload, users, tab, summary.OwnerNames})
}

func (c *Controller) TrashShare(w http.ResponseWriter, r *http.Request) error {
	if err := c.shareSvc.TrashShare(r.PathValue("id"), session.UserFromContext(r.Context())); err != nil {
		return err
	}
	c.flash.Success(w, "share trashed")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) RestoreShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	actor := session.UserFromContext(r.Context())
	toEdit := r.PostFormValue("redirect") == "edit"

	if err := c.shareSvc.RecoverShare(id, actor); err != nil {
		return err
	}

	c.flash.Success(w, "share restored")
	if toEdit {
		http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/trash", http.StatusSeeOther)
	}
	return nil
}

func (c *Controller) DeleteShares(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.Delete(session.UserFromContext(r.Context()), r.PostForm["shareID"]...); err != nil {
		return err
	}
	c.flash.Success(w, "shares deleted")
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
	return nil
}

func (c *Controller) DeleteFile(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := c.shareSvc.DeleteFile(id, r.PathValue("fileID"), session.UserFromContext(r.Context())); err != nil {
		return err
	}
	c.flash.Success(w, "file deleted")
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusSeeOther)
	return nil
}

func (c *Controller) AddOwner(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.AddOwner(id, session.UserFromContext(r.Context()), r.PostForm.Get("userID")); err != nil {
		return err
	}
	c.flash.Success(w, "owner added")
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusSeeOther)
	return nil
}

func (c *Controller) RemoveOwner(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := c.shareSvc.RemoveOwner(id, session.UserFromContext(r.Context()), r.PathValue("ownerID")); err != nil {
		return err
	}
	c.flash.Success(w, "owner removed")
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=owners", http.StatusSeeOther)
	return nil
}

func (c *Controller) UpdateShareSettings(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.SetPassword(id, session.UserFromContext(r.Context()), r.PostForm.Get("password"), r.PostForm.Get("removePassword") == "on"); err != nil {
		return err
	}
	c.flash.Success(w, "settings saved")
	http.Redirect(w, r, "/shares/"+id+"/edit?tab=settings", http.StatusSeeOther)
	return nil
}

func (c *Controller) SetVisibility(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.SetVisibility(id, session.UserFromContext(r.Context()), share.Visibility(r.PostForm.Get("visibility"))); err != nil {
		return err
	}
	c.flash.Success(w, "visibility updated")
	http.Redirect(w, r, "/shares/"+id+"/edit", http.StatusSeeOther)
	return nil
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
