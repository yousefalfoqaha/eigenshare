package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"eigenshare/internal/session"
	"eigenshare/internal/share"
)

func (c *Controller) CreateShare(w http.ResponseWriter, r *http.Request) error {
	var req share.ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return share.ErrInvalidRequest
	}

	sh, err := c.shareSvc.RequestShare(req, session.UserFromContext(r.Context()))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(sh)
}

func (c *Controller) AddFilesToShare(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return share.ErrInvalidRequest
	}

	newFiles, err := c.shareSvc.UploadFiles(r.PathValue("id"), req.Files, session.UserFromContext(r.Context()))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(newFiles)
}

func (c *Controller) GetUploadOffset(w http.ResponseWriter, r *http.Request) error {
	actor := session.UserFromContext(r.Context())
	sh, err := c.shareSvc.GetShare(r.PathValue("id"), actor)
	if err != nil {
		return err
	}

	state, err := c.shareSvc.GetFileUploadState(sh, actor, r.PathValue("fileID"))
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
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return share.ErrInvalidRequest
	}

	bytesWritten, err := c.shareSvc.StreamFile(
		r.PathValue("id"), r.PathValue("fileID"), offset, r.Body, session.UserFromContext(r.Context()))
	if err != nil {
		if conflict, ok := errors.AsType[*share.ChunkOffsetConflictError](err); ok {
			w.Header().Set("Upload-Offset", strconv.FormatInt(conflict.CurrentOffset, 10))
		}
		return err
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *Controller) DeleteShareFile(w http.ResponseWriter, r *http.Request) error {
	if err := c.shareSvc.DeleteFile(r.PathValue("id"), r.PathValue("fileID"), session.UserFromContext(r.Context())); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
