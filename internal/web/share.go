package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
)

const (
	unlockShareCookiePrefix = "share_unlock_"
	shareUnlockTTL          = 1 * 24 * time.Hour
)

type shareKey struct{}

func shareFromContext(r *http.Request) *share.Share {
	sh, _ := r.Context().Value(shareKey{}).(*share.Share)
	return sh
}

func loadShare(shareSvc *share.ShareService) middleware {
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

var requireViewableShare middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanView(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return next(w, r)
	}
}

var requireManageableShare middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		sh := shareFromContext(r)
		if sh == nil || !sh.CanManage(userFromContext(r)) {
			return share.ErrShareNotFound
		}
		return next(w, r)
	}
}

func requireUnlockedShare(jar *cookies) middleware {
	return func(next handler) handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			sh := shareFromContext(r)
			if sh == nil {
				return share.ErrShareNotFound
			}
			if !sh.RequiresPassword(userFromContext(r)) {
				return next(w, r)
			}

			val, ok := jar.read(r, unlockShareCookiePrefix+sh.ID)
			id, fp, ok := strings.Cut(val, ":")
			if ok && id == sh.ID && fp == security.Fingerprint(sh.PasswordHash) {
				return next(w, r)
			}

			if ok {
				jar.write(w, unlockShareCookiePrefix+id, "", "/", -1)
			}

			http.Redirect(w, r, "/s/"+sh.Handle+"/unlock?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return nil
		}
	}
}

func (c *Controller) sharePage(w http.ResponseWriter, r *http.Request) error {
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

func (c *Controller) unlockPage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if sh == nil {
		return share.ErrShareNotFound
	}
	return c.templs.renderPage(w, r, http.StatusOK, "share_unlock.html", &struct {
		Handle   string
		Redirect string
	}{sh.Handle, r.URL.Query().Get("redirect")})
}

func (c *Controller) unlockShare(w http.ResponseWriter, r *http.Request) error {
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
		return errInvalidRequest
	}

	if !security.VerifyPassword(sh.PasswordHash, r.PostForm.Get("password")) {
		return security.ErrWrongPassword
	}

	c.cookies.write(w, unlockShareCookiePrefix+sh.ID, fmt.Sprintf("%s:%s", sh.ID, security.Fingerprint(sh.PasswordHash)), "/", shareUnlockTTL)
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

func (c *Controller) downloadAll(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(sh.Handle+".zip"))
	return c.shareSvc.DownloadAll(sh, w)
}

func (c *Controller) downloadFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	file, filePath, err := c.shareSvc.DownloadFile(sh, r.PathValue("fileID"))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Disposition", contentDisposition(file.Name))
	http.ServeFile(w, r, filePath)
	return nil
}

func (c *Controller) sharesPage(w http.ResponseWriter, r *http.Request) error {
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

func (c *Controller) newSharePage(w http.ResponseWriter, r *http.Request) error {
	return c.templs.renderPage(w, r, http.StatusOK, "new_share.html", nil)
}

func (c *Controller) createShare(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return errInvalidRequest
	}

	expiryMinutes, _ := strconv.Atoi(r.PostForm.Get("expiryMinutes"))
	maxSizeMB, _ := strconv.ParseInt(r.PostForm.Get("maxSizeMB"), 10, 64)
	if maxSizeMB < 0 || maxSizeMB > math.MaxInt64/(1024*1024) {
		return share.ErrInvalidMaxBytes
	}

	req := share.ShareRequest{
		Handle:        r.PostForm.Get("handle"),
		ExpiryMinutes: expiryMinutes,
		MaxBytes:      maxSizeMB * 1024 * 1024,
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

func (c *Controller) trashPage(w http.ResponseWriter, r *http.Request) error {
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

func (c *Controller) editSharePage(w http.ResponseWriter, r *http.Request) error {
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
		Share       *share.Share
		Upload      map[string]share.FileUploadState
		Tab         string
		ChunkSizeMB int
	}{sh, states, tab, c.chunkSizeMB})
}

func (c *Controller) trashShare(w http.ResponseWriter, r *http.Request) error {
	if err := c.shareSvc.TrashShare(shareFromContext(r)); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "share trashed")
	http.Redirect(w, r, "/shares", http.StatusSeeOther)
	return nil
}

func (c *Controller) restoreShare(w http.ResponseWriter, r *http.Request) error {
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

func (c *Controller) deleteShares(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return errInvalidRequest
	}
	if err := c.shareSvc.Delete(userFromContext(r), r.PostForm["shareID"]...); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "share deleted")
	http.Redirect(w, r, "/trash", http.StatusSeeOther)
	return nil
}

func (c *Controller) deleteFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := c.shareSvc.DeleteFile(sh, r.PathValue("fileID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "file deleted")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	return nil
}

func (c *Controller) addOwner(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := r.ParseForm(); err != nil {
		return errInvalidRequest
	}
	if err := c.shareSvc.AddOwner(sh, r.PostForm.Get("userID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "owner added")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=owners", http.StatusSeeOther)
	return nil
}

func (c *Controller) removeOwner(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := c.shareSvc.RemoveOwner(sh, r.PathValue("ownerID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "owner removed")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=owners", http.StatusSeeOther)
	return nil
}

func (c *Controller) updateShareSecurity(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := r.ParseForm(); err != nil {
		return errInvalidRequest
	}
	if err := c.shareSvc.SetPassword(sh, r.PostForm.Get("password"), r.PostForm.Get("removePassword") == "on"); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "security saved")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=security", http.StatusSeeOther)
	return nil
}

func (c *Controller) setVisibility(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)

	if err := r.ParseForm(); err != nil {
		return errInvalidRequest
	}
	if err := c.shareSvc.SetVisibility(sh, share.Visibility(r.PostForm.Get("visibility"))); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "visibility updated")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit", http.StatusSeeOther)
	return nil
}

func (c *Controller) addFilesToShare(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errInvalidRequest
	}

	newFiles, err := c.shareSvc.UploadFiles(shareFromContext(r), req.Files, nil)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
}

func (c *Controller) getUploadOffset(w http.ResponseWriter, r *http.Request) error {
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

func (c *Controller) streamFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}

	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return errInvalidRequest
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

func (c *Controller) deleteShareFile(w http.ResponseWriter, r *http.Request) error {
	if err := c.shareSvc.DeleteFile(shareFromContext(r), r.PathValue("fileID")); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
