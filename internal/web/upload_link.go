package web

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"eigenshare/internal/share"
)

const (
	uploadFileCookiePrefix = "upload_file_"
	uploadFileCookieTTL    = 7 * 24 * time.Hour
)

func formatUploadFileCookie(linkID, fileID string) string {
	return uploadFileCookiePrefix + linkID + "_" + fileID
}

func SetUploadFile(jar *cookies, w http.ResponseWriter, link *share.UploadLink, fileID string) {
	ttl := uploadFileCookieTTL
	if link.ExpiresAt != nil {
		if remaining := time.Until(*link.ExpiresAt); remaining > 0 {
			ttl = remaining
		}
	}

	jar.write(w, formatUploadFileCookie(link.ID, fileID), "1", "/", ttl)
}

func ClearUploadFile(jar *cookies, w http.ResponseWriter, link *share.UploadLink, fileID string) {
	jar.write(w, formatUploadFileCookie(link.ID, fileID), "", "/", -1)
}

func GetUploadFileIDs(jar *cookies, r *http.Request, linkID string) []string {
	prefix := uploadFileCookiePrefix + linkID + "_"

	var fileIDs []string
	for _, cookie := range r.Cookies() {
		if !strings.HasPrefix(cookie.Name, prefix) {
			continue
		}
		fileID := strings.TrimPrefix(cookie.Name, prefix)
		if fileID == "" {
			continue
		}
		if v, ok := jar.read(r, cookie.Name); !ok || v != "1" {
			continue
		}
		fileIDs = append(fileIDs, fileID)
	}

	return fileIDs
}

type uploadLinkKey struct{}

func uploadLinkFromContext(r *http.Request) *share.UploadLink {
	link, _ := r.Context().Value(uploadLinkKey{}).(*share.UploadLink)
	return link
}

var LoadUploadLink middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if sh := shareFromContext(r); sh != nil {
			if link, err := sh.UploadLink(r.URL.Query().Get("token")); err == nil {
				ctx := context.WithValue(r.Context(), uploadLinkKey{}, link)
				return next(w, r.WithContext(ctx))
			}
		}

		return next(w, r)
	}
}

var RequireUploadLink middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		link := uploadLinkFromContext(r)
		if link == nil {
			return share.ErrUploadLinkNotFound
		}
		if link.ExpiresAt != nil && time.Now().UTC().After(*link.ExpiresAt) {
			return share.ErrUploadLinkNotFound
		}
		return next(w, r)
	}
}

func (c *Controller) UploadLinkPage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	link := uploadLinkFromContext(r)

	states, err := c.shareSvc.GetShareUploadState(sh)
	if err != nil {
		return err
	}

	var seedFiles []share.File
	fileIDs := GetUploadFileIDs(c.cookies, r, link.ID)
	fileIDSet := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		fileIDSet[id] = true
	}
	for _, f := range sh.Files {
		if !fileIDSet[f.ID] {
			continue
		}
		if f.Complete(states[f.ID].Written) {
			ClearUploadFile(c.cookies, w, link, f.ID)
			continue
		}
		seedFiles = append(seedFiles, f)
	}
	for id := range fileIDSet {
		if _, err := sh.File(id); err != nil {
			ClearUploadFile(c.cookies, w, link, id)
		}
	}

	remaining := link.Remaining()

	return c.templs.renderPage(w, r, http.StatusOK, "upload_link.html", &struct {
		Share     *share.Share
		Link      *share.UploadLink
		Upload    map[string]share.FileUploadState
		SeedFiles []share.File
		Remaining int64
	}{sh, link, states, seedFiles, remaining})
}

func (c *Controller) CreateUploadLink(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := r.ParseForm(); err != nil {
		return ErrInvalidRequest
	}

	expiryMinutes, _ := strconv.Atoi(r.PostForm.Get("expiryMinutes"))
	maxSizeMB, _ := strconv.ParseInt(r.PostForm.Get("maxSizeMB"), 10, 64)
	if maxSizeMB < 0 || maxSizeMB > math.MaxInt64/(1024*1024) {
		return share.ErrInvalidMaxBytes
	}

	req := share.UploadLinkRequest{
		ExpiryMinutes: expiryMinutes,
		MaxBytes:      maxSizeMB * 1024 * 1024,
	}

	if _, err := c.shareSvc.CreateUploadLink(sh, req); err != nil {
		return err
	}

	c.cookies.SetFlash(w, "success", "upload link created")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=uploads", http.StatusSeeOther)
	return nil
}

func (c *Controller) RevokeUploadLink(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	if err := c.shareSvc.RevokeUploadLink(sh, r.PathValue("linkID")); err != nil {
		return err
	}
	c.cookies.SetFlash(w, "success", "upload link revoked")
	http.Redirect(w, r, "/shares/"+sh.ID+"/edit?tab=uploads", http.StatusSeeOther)
	return nil
}

func (c *Controller) AddFilesViaUploadLink(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	link := uploadLinkFromContext(r)

	var body struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return ErrInvalidRequest
	}

	newFiles, err := c.shareSvc.UploadFiles(sh, body.Files, link)
	if err != nil {
		return err
	}

	for _, f := range newFiles {
		SetUploadFile(c.cookies, w, link, f.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
}

func (c *Controller) GetUploadLinkFileOffset(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	link := uploadLinkFromContext(r)

	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}
	if !slices.Contains(GetUploadFileIDs(c.cookies, r, link.ID), f.ID) {
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

func (c *Controller) StreamUploadLinkFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	link := uploadLinkFromContext(r)

	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}
	if !slices.Contains(GetUploadFileIDs(c.cookies, r, link.ID), f.ID) {
		return share.ErrFileNotFound
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
		ClearUploadFile(c.cookies, w, link, f.ID)
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) DeleteUploadLinkFile(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	link := uploadLinkFromContext(r)

	f, err := sh.File(r.PathValue("fileID"))
	if err != nil {
		return err
	}
	if !slices.Contains(GetUploadFileIDs(c.cookies, r, link.ID), f.ID) {
		return share.ErrFileNotFound
	}

	if err := c.shareSvc.DeleteFile(sh, f.ID); err != nil {
		return err
	}

	ClearUploadFile(c.cookies, w, link, f.ID)

	w.WriteHeader(http.StatusNoContent)
	return nil
}
