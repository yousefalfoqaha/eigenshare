package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
)

const unlockShareCookiePrefix = "share_unlock_"

type shareKey struct{}

func shareFromContext(r *http.Request) *share.Share {
	sh, _ := r.Context().Value(shareKey{}).(*share.Share)
	return sh
}

func SetUnlock(jar *cookies, w http.ResponseWriter, sh *share.Share) {
	jar.write(w, unlockShareCookiePrefix+sh.ID, fmt.Sprintf("%s:%s", sh.ID, security.Fingerprint(sh.PasswordHash)), "/", share.UnlockTTL)
}

func GetUnlock(jar *cookies, r *http.Request, shareID string) (id, fp string, ok bool) {
	val, ok := jar.read(r, unlockShareCookiePrefix+shareID)
	if !ok {
		return "", "", false
	}
	id, fp, ok = strings.Cut(val, ":")
	return id, fp, ok
}

func ClearUnlock(jar *cookies, w http.ResponseWriter, id string) {
	jar.write(w, unlockShareCookiePrefix+id, "", "/", -1)
}

func LoadShare(shareSvc *share.ShareService) middleware {
	return func(next handler) handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			var (
				sh  *share.Share
				err error
			)
			if handle := r.PathValue("handle"); handle != "" {
				sh, err = shareSvc.GetShareByHandle(handle)
			} else if id := r.PathValue("id"); id != "" {
				sh, err = shareSvc.GetShare(id)
			}
			if err != nil {
				if !errors.Is(err, share.ErrShareNotFound) {
					return err
				}
				return next(w, r)
			}
			if sh == nil {
				return next(w, r)
			}

			ctx := context.WithValue(r.Context(), shareKey{}, sh)
			return next(w, r.WithContext(ctx))
		}
	}
}

var RequireViewableShare middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanView(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return next(w, r)
	}
}

var RequireManageable middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanManage(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return next(w, r)
	}
}

func RequireUnlockedShare(jar *cookies) middleware {
	return func(next handler) handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			sh := shareFromContext(r)
			if sh == nil {
				return share.ErrShareNotFound
			}
			if !sh.RequiresPassword(userFromContext(r)) {
				return next(w, r)
			}

			id, fp, ok := GetUnlock(jar, r, sh.ID)
			if ok && id == sh.ID && fp == security.Fingerprint(sh.PasswordHash) {
				return next(w, r)
			}

			if ok {
				ClearUnlock(jar, w, sh.ID)
			}

			http.Redirect(w, r, "/s/"+sh.Handle+"/unlock?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return nil
		}
	}
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
		return ErrInvalidRequest
	}

	if !security.VerifyPassword(sh.PasswordHash, r.PostForm.Get("password")) {
		return security.ErrWrongPassword
	}

	SetUnlock(c.cookies, w, sh)
	c.cookies.SetFlash(w, "success", "share unlocked")
	http.Redirect(w, r, redirectAfterUnlock(r.PostForm.Get("redirect"), sh), http.StatusSeeOther)
	return nil
}

func redirectAfterUnlock(redirect string, sh *share.Share) string {
	if redirect == "" || !strings.HasPrefix(redirect, "/") || strings.HasPrefix(redirect, "//") {
		return "/s/" + sh.Handle
	}
	return redirect
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
		return ErrInvalidRequest
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
		return ErrInvalidRequest
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
		return ErrInvalidRequest
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
		return ErrInvalidRequest
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
		return ErrInvalidRequest
	}
	if err := c.shareSvc.SetVisibility(sh, share.Visibility(r.PostForm.Get("visibility"))); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "visibility updated")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	return nil
}

func (c *Controller) AddFilesToShare(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return ErrInvalidRequest
	}

	newFiles, err := c.shareSvc.UploadFiles(shareFromContext(r), req.Files, nil)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
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
		return ErrInvalidRequest
	}

	bytesWritten, err := c.shareSvc.StreamFile(sh, f.ID, offset, r.Body)
	if err != nil {
		if errors.Is(err, share.ErrInvalidChunkOffset) {
			w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
		}
		return err
	}

	if f.Complete(bytesWritten) {
		c.cookies.SetFlash(w, "success", "files uploaded")
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
