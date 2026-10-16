package web

import (
	"encoding/json"
	"errors"
	"math"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func (c *Controller) Index(w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, "/shares", http.StatusFound)
	return nil
}

func (c *Controller) LoginPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "login.html", nil)
}

func (c *Controller) LoginSubmit(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}

	u, err := c.userSvc.GetUserByUsername(r.PostForm.Get("username"))
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return ErrInvalidCredentials
		}
		return err
	}

	if !security.VerifyPassword(u.PasswordHash, r.PostForm.Get("password")) {
		return ErrInvalidCredentials
	}

	c.cookies.SetSession(w, u)
	c.cookies.SetFlash(w, "success", "logged in")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) Logout(w http.ResponseWriter, r *http.Request) error {
	c.cookies.ClearSession(w)
	c.cookies.SetFlash(w, "success", "logged out")
	http.Redirect(w, r, "/login", http.StatusSeeOther)

	return nil
}

func (c *Controller) SharePage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	states, err := c.shareSvc.GetShareUploadState(sh)
	if err != nil {
		return err
	}

	return c.templs.renderPage(w, r, http.StatusOK, "share.html", &struct {
		Share  *share.Share
		Upload map[string]share.FileUploadState
	}{sh, states})
}

func (c *Controller) UnlockPage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if sh == nil {
		return share.ErrShareNotFound
	}
	return c.templs.renderPage(w, r, http.StatusOK, "share_unlock.html", &struct {
		Handle   string
		Redirect string
	}{sh.Handle, r.URL.Query().Get("redirect")})
}

func (c *Controller) UnlockShare(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if sh == nil {
		return share.ErrShareNotFound
	}
	actor := userFromContext(r)

	if !sh.RequiresPassword(actor) {
		http.Redirect(w, r, "/s/"+sh.Handle, http.StatusSeeOther)
		return nil
	}

	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}

	if !security.VerifyPassword(sh.PasswordHash, r.PostForm.Get("password")) {
		return security.ErrWrongPassword
	}

	c.cookies.SetUnlock(w, sh)
	c.cookies.SetFlash(w, "success", "share unlocked")
	http.Redirect(w, r, redirectAfterUnlock(r.PostForm.Get("redirect"), sh), http.StatusSeeOther)
	return nil
}

// redirectAfterUnlock returns a safe same-site path to return to after
// unlocking, defaulting to the share page.
func redirectAfterUnlock(redirect string, sh *share.Share) string {
	if redirect == "" || !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		return "/s/" + sh.Handle
	}
	return redirect
}

func (c *Controller) UsersPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "users.html", nil)
}

func (c *Controller) NewUserPage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "user_form.html", nil)
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
	c.cookies.SetFlash(w, "success", "user created")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) EditUserPage(w http.ResponseWriter, r *http.Request) error {
	u, err := c.userSvc.GetUser(r.PathValue("id"))
	if err != nil {
		return err
	}
	return c.templs.renderPage(w, r, http.StatusOK, "user_form.html", u)
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
	if _, err := c.userSvc.UpdateUser(r.PathValue("id"), userFromContext(r).ID, req); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "user updated")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) DeleteUser(w http.ResponseWriter, r *http.Request) error {
	if err := c.userSvc.DeleteUser(r.PathValue("id"), userFromContext(r).ID); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "user deleted")
	http.Redirect(w, r, "/users", http.StatusSeeOther)
	return nil
}

func (c *Controller) SharesPage(w http.ResponseWriter, r *http.Request) error {
	actor := userFromContext(r)

	shares, err := c.shareSvc.GetShares(actor)
	if err != nil {
		return err
	}

	rows := make([]struct {
		Share  *share.Share
		Upload map[string]share.FileUploadState
	}, len(shares))
	for i, sh := range shares {
		states, err := c.shareSvc.GetShareUploadState(sh)
		if err != nil {
			return err
		}
		rows[i].Share = sh
		rows[i].Upload = states
	}

	return c.templs.renderPage(w, r, http.StatusOK, "shares.html", &struct {
		Shares []struct {
			Share  *share.Share
			Upload map[string]share.FileUploadState
		}
	}{rows})
}

func (c *Controller) NewSharePage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "new_share.html", nil)
}

func (c *Controller) CreateShare(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}

	expiryMinutes, _ := strconv.Atoi(r.PostForm.Get("expiryMinutes"))

	req := share.ShareRequest{
		Handle:        r.PostForm.Get("handle"),
		ExpiryMinutes: expiryMinutes,
		Visibility:    share.Visibility(r.PostForm.Get("visibility")),
		Password:      r.PostForm.Get("password"),
	}

	sh, err := c.shareSvc.RequestShare(req, userFromContext(r))
	if err != nil {
		return err
	}

	c.cookies.SetFlash(w, "success", "share created")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	return nil
}

func (c *Controller) TrashPage(w http.ResponseWriter, r *http.Request) error {
	actor := userFromContext(r)

	shares, err := c.shareSvc.GetTrashedShares(actor)
	if err != nil {
		return err
	}

	rows := make([]struct {
		Share  *share.Share
		Upload map[string]share.FileUploadState
	}, len(shares))
	for i, sh := range shares {
		states, err := c.shareSvc.GetShareUploadState(sh)
		if err != nil {
			return err
		}
		rows[i].Share = sh
		rows[i].Upload = states
	}

	return c.templs.renderPage(w, r, http.StatusOK, "trash.html", &struct {
		Shares []struct {
			Share  *share.Share
			Upload map[string]share.FileUploadState
		}
	}{rows})
}

func (c *Controller) EditSharePage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	states, err := c.shareSvc.GetShareUploadState(sh)
	if err != nil {
		return err
	}

	tab := r.URL.Query().Get("tab")
	switch tab {
	case "files", "uploads", "owners", "security":
	default:
		tab = "files"
	}

	return c.templs.renderPage(w, r, http.StatusOK, "edit_share.html", &struct {
		Share  *share.Share
		Upload map[string]share.FileUploadState
		Tab    string
	}{sh, states, tab})
}

func (c *Controller) UploadRequestPage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	req := uploadRequestFromContext(r)

	states, err := c.shareSvc.GetShareUploadState(sh)
	if err != nil {
		return err
	}

	var seedFiles []share.File
	fileIDs := c.cookies.GetUploadFileIDs(r, req.ID)
	fileIDSet := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		fileIDSet[id] = true
	}
	for _, f := range sh.Files {
		if !fileIDSet[f.ID] {
			continue
		}
		if states[f.ID].Written >= f.ExpectedSize {
			c.cookies.ClearUploadFile(w, req, f.ID)
			continue
		}
		seedFiles = append(seedFiles, f)
	}
	for id := range fileIDSet {
		if _, err := sh.File(id); err != nil {
			c.cookies.ClearUploadFile(w, req, id)
		}
	}

	remaining := req.MaxBytes - req.BytesUsed
	if remaining < 0 {
		remaining = 0
	}

	return c.templs.renderPage(w, r, http.StatusOK, "upload_request.html", &struct {
		Share     *share.Share
		Request   *share.UploadRequest
		Upload    map[string]share.FileUploadState
		SeedFiles []share.File
		Remaining int64
	}{sh, req, states, seedFiles, remaining})
}

func (c *Controller) CreateUploadRequest(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}

	expiryMinutes, _ := strconv.Atoi(r.PostForm.Get("expiryMinutes"))
	maxSizeMB, _ := strconv.ParseInt(r.PostForm.Get("maxSizeMB"), 10, 64)
	if maxSizeMB < 0 || maxSizeMB > math.MaxInt64/(1024*1024) {
		return share.ErrInvalidMaxBytes
	}

	req := share.UploadRequestRequest{
		ExpiryMinutes: expiryMinutes,
		MaxBytes:      maxSizeMB * 1024 * 1024,
	}

	if _, err := c.shareSvc.CreateUploadRequest(sh, req); err != nil {
		return err
	}

	c.cookies.SetFlash(w, "success", "upload request created")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=uploads", http.StatusSeeOther)
	return nil
}

func (c *Controller) RevokeUploadRequest(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := c.shareSvc.RevokeUploadRequest(sh, r.PathValue("requestID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "upload request revoked")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=uploads", http.StatusSeeOther)
	return nil
}

func (c *Controller) TrashShare(w http.ResponseWriter, r *http.Request) error {
	if err := c.shareSvc.TrashShare(shareFromContext(r)); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "share trashed")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) RestoreShare(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	toEdit := r.PostFormValue("redirect") == "edit"

	if err := c.shareSvc.RecoverShare(sh); err != nil {
		return err
	}

	c.cookies.SetFlash(w, "success", "share restored")
	if toEdit {
		http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/trash", http.StatusSeeOther)
	}
	return nil
}

func (c *Controller) DeleteShares(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.Delete(userFromContext(r), r.PostForm["shareID"]...); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "share deleted")
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
	return nil
}

func (c *Controller) DeleteFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := c.shareSvc.DeleteFile(sh, r.PathValue("fileID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "file deleted")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	return nil
}

func (c *Controller) AddOwner(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.AddOwner(sh, r.PostForm.Get("userID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "owner added")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=owners", http.StatusSeeOther)
	return nil
}

func (c *Controller) RemoveOwner(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := c.shareSvc.RemoveOwner(sh, r.PathValue("ownerID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "owner removed")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=owners", http.StatusSeeOther)
	return nil
}

func (c *Controller) UpdateShareSecurity(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.SetPassword(sh, r.PostForm.Get("password"), r.PostForm.Get("removePassword") == "on"); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "security saved")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=security", http.StatusSeeOther)
	return nil
}

func (c *Controller) SetVisibility(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	if err := r.ParseForm(); err != nil {
		return share.ErrInvalidRequest
	}
	if err := c.shareSvc.SetVisibility(sh, share.Visibility(r.PostForm.Get("visibility"))); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "visibility updated")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	return nil
}

func contentDisposition(filename string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}

func (c *Controller) DownloadAll(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(sh.Handle+".zip"))
	return c.shareSvc.DownloadAll(sh, w)
}

func (c *Controller) DownloadFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	file, filePath, err := c.shareSvc.DownloadFile(sh, r.PathValue("fileID"))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Disposition", contentDisposition(file.Name))
	http.ServeFile(w, r, filePath)
	return nil
}

func (c *Controller) AddFilesToShare(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return share.ErrInvalidRequest
	}

	newFiles, err := c.shareSvc.UploadFiles(shareFromContext(r), req.Files, nil)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
}

func (c *Controller) AddFilesViaUploadRequest(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	req := uploadRequestFromContext(r)

	var body struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return share.ErrInvalidRequest
	}

	newFiles, err := c.shareSvc.UploadFiles(sh, body.Files, req)
	if err != nil {
		return err
	}

	for _, f := range newFiles {
		c.cookies.SetUploadFile(w, req, f.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
}

func (c *Controller) GetUploadRequestFileOffset(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	req := uploadRequestFromContext(r)

	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}
	if !slices.Contains(c.cookies.GetUploadFileIDs(r, req.ID), f.ID) {
		return share.ErrFileNotFound
	}

	state, err := c.shareSvc.GetFileUploadState(sh, f.ID)
	if err != nil {
		return err
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(state.Written, 10))
	if state.Locked {
		w.Header().Set("Upload-Locked", "1")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) StreamUploadRequestFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	req := uploadRequestFromContext(r)

	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}
	if !slices.Contains(c.cookies.GetUploadFileIDs(r, req.ID), f.ID) {
		return share.ErrFileNotFound
	}

	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return share.ErrInvalidRequest
	}

	bytesWritten, err := c.shareSvc.StreamFile(sh, f.ID, offset, r.Body)
	if err != nil {
		if errors.Is(err, share.ErrInvalidChunkOffset) {
			w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
		}
		return err
	}

	if bytesWritten >= f.ExpectedSize {
		c.cookies.SetFlash(w, "success", "files uploaded")
		c.cookies.ClearUploadFile(w, req, f.ID)
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) DeleteUploadRequestFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	req := uploadRequestFromContext(r)

	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}
	if !slices.Contains(c.cookies.GetUploadFileIDs(r, req.ID), f.ID) {
		return share.ErrFileNotFound
	}

	if err := c.shareSvc.DeleteFile(sh, f.ID); err != nil {
		return err
	}

	c.cookies.ClearUploadFile(w, req, f.ID)

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) GetUploadOffset(w http.ResponseWriter, r *http.Request) error {
	state, err := c.shareSvc.GetFileUploadState(shareFromContext(r), r.PathValue("fileID"))
	if err != nil {
		return err
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(state.Written, 10))
	if state.Locked {
		w.Header().Set("Upload-Locked", "1")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) StreamFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}

	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return share.ErrInvalidRequest
	}

	bytesWritten, err := c.shareSvc.StreamFile(sh, f.ID, offset, r.Body)
	if err != nil {
		if errors.Is(err, share.ErrInvalidChunkOffset) {
			w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
		}
		return err
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) DeleteShareFile(w http.ResponseWriter, r *http.Request) error {
	if err := c.shareSvc.DeleteFile(shareFromContext(r), r.PathValue("fileID")); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
