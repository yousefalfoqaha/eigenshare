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

// uploadFileCookiePrefix names the cookies holding files registered through
// an upload request. Each cookie carries one file id.
const uploadFileCookiePrefix = "upload_file_"

// uploadFileCookieTTL is how long an upload file cookie lives when the
// request has no expiry. Registration and page loads refresh the cookies
// they still care about.
const uploadFileCookieTTL = 365 * 24 * time.Hour

func uploadFileCookieName(requestID, fileID string) string {
	return uploadFileCookiePrefix + requestID + "_" + fileID
}

// SetUploadFile remembers a file registered through the upload request.
func SetUploadFile(jar *cookies, w http.ResponseWriter, req *share.UploadRequest, fileID string) {
	ttl := uploadFileCookieTTL
	if req.ExpiresAt != nil {
		if remaining := time.Until(req.ExpiresAt.Time()); remaining > 0 {
			ttl = remaining
		}
	}

	jar.write(w, uploadFileCookieName(req.ID, fileID), "1", "/", ttl)
}

// ClearUploadFile forgets a file previously registered through the upload
// request.
func ClearUploadFile(jar *cookies, w http.ResponseWriter, req *share.UploadRequest, fileID string) {
	jar.write(w, uploadFileCookieName(req.ID, fileID), "", "/", -1)
}

// GetUploadFileIDs returns the files registered through the upload request
// by this client, or nil when none are remembered. Only cookies with a
// valid signature are considered.
func GetUploadFileIDs(jar *cookies, r *http.Request, requestID string) []string {
	prefix := uploadFileCookiePrefix + requestID + "_"

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

type uploadRequestKey struct{}

func uploadRequestFromContext(r *http.Request) *share.UploadRequest {
	req, _ := r.Context().Value(uploadRequestKey{}).(*share.UploadRequest)
	return req
}

// LoadUploadRequest loads the upload request identified by the token query
// parameter into the request context. Requests that cannot be resolved are
// left nil so guards can decide how to respond.
var LoadUploadRequest middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if sh := shareFromContext(r); sh != nil {
			if req, err := sh.UploadRequest(r.URL.Query().Get("token")); err == nil {
				ctx := context.WithValue(r.Context(), uploadRequestKey{}, req)
				return next(w, r.WithContext(ctx))
			}
		}

		return next(w, r)
	}
}

// RequireUploadRequest requires a valid, unexpired upload request resolved
// from the token query parameter.
var RequireUploadRequest middleware = func(next handler) handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		req := uploadRequestFromContext(r)
		if req == nil {
			return share.ErrUploadRequestNotFound
		}
		if req.ExpiresAt != nil && time.Now().After(req.ExpiresAt.Time()) {
			return share.ErrUploadRequestNotFound
		}
		return next(w, r)
	}
}

func (c *Controller) UploadRequestPage(w http.ResponseWriter, r *http.Request) error {
	sh := shareFromContext(r)
	req := uploadRequestFromContext(r)

	states, err := c.shareSvc.GetShareUploadState(sh)
	if err != nil {
		return err
	}

	var seedFiles []share.File
	fileIDs := GetUploadFileIDs(c.cookies, r, req.ID)
	fileIDSet := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		fileIDSet[id] = true
	}
	for _, f := range sh.Files {
		if !fileIDSet[f.ID] {
			continue
		}
		if f.Complete(states[f.ID].Written) {
			ClearUploadFile(c.cookies, w, req, f.ID)
			continue
		}
		seedFiles = append(seedFiles, f)
	}
	for id := range fileIDSet {
		if _, err := sh.File(id); err != nil {
			ClearUploadFile(c.cookies, w, req, id)
		}
	}

	remaining := req.Remaining()

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
		SetUploadFile(c.cookies, w, req, f.ID)
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
	if !slices.Contains(GetUploadFileIDs(c.cookies, r, req.ID), f.ID) {
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
	if !slices.Contains(GetUploadFileIDs(c.cookies, r, req.ID), f.ID) {
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

	if f.Complete(bytesWritten) {
		c.cookies.SetFlash(w, "success", "files uploaded")
		ClearUploadFile(c.cookies, w, req, f.ID)
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
	if !slices.Contains(GetUploadFileIDs(c.cookies, r, req.ID), f.ID) {
		return share.ErrFileNotFound
	}

	if err := c.shareSvc.DeleteFile(sh, f.ID); err != nil {
		return err
	}

	ClearUploadFile(c.cookies, w, req, f.ID)

	w.WriteHeader(http.StatusNoContent)
	return nil
}
