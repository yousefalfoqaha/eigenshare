package web

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"eigenshare/internal/session"
	"eigenshare/internal/share"
)

// UploadRouter owns every JSON endpoint of the resumable-upload API. It has
// no access to templates, cookies or redirects — only byte streams and the
// share service.
type UploadRouter struct {
	Shares *share.ShareService
}

func (u *UploadRouter) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /shares/{id}/files", handleAPI(u.RegisterFiles))
	mux.Handle("HEAD /shares/{id}/files/{fileID}/stream", handleAPI(u.GetUploadOffset))
	mux.Handle("PUT /shares/{id}/files/{fileID}/stream", handleAPI(u.StreamFile))
	mux.Handle("DELETE /shares/{id}/files/{fileID}", handleAPI(u.DeleteFile))

	return requireSession(writeAPIUnauthorized)(mux)
}

// UploadRoutes wires the upload API onto a mux, behind session auth.
func UploadRoutes(shares *share.ShareService) http.Handler {
	return (&UploadRouter{Shares: shares}).Routes()
}

func handleAPI(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			status, msg := mapError(err)
			if status >= http.StatusInternalServerError {
				log.Printf("INTERNAL ERROR: %v", err)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": msg})
		}
	}
}

func (u *UploadRouter) RegisterFiles(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return share.ErrInvalidRequest
	}

	newFiles, err := u.Shares.UploadFiles(r.PathValue("id"), req.Files, session.UserFromContext(r.Context()))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
}

func (u *UploadRouter) GetUploadOffset(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())
	sh, err := u.Shares.GetShare(r.PathValue("id"), actor)
	if err != nil {
		return err
	}

	state, err := u.Shares.GetFileUploadState(sh, actor, r.PathValue("fileID"))
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

func (u *UploadRouter) StreamFile(w http.ResponseWriter, r *http.Request) error {
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return share.ErrInvalidRequest
	}

	bytesWritten, err := u.Shares.StreamFile(
		r.PathValue("id"), r.PathValue("fileID"), offset, r.Body, session.UserFromContext(r.Context()))
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

func (u *UploadRouter) DeleteFile(w http.ResponseWriter, r *http.Request) error {
	if err := u.Shares.DeleteFile(r.PathValue("id"), r.PathValue("fileID"), session.UserFromContext(r.Context())); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
