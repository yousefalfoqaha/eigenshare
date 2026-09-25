package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"eigenshare/internal/apperr"
	"eigenshare/internal/auth"
	"eigenshare/internal/share"
)

func (app *App) createShare(w http.ResponseWriter, r *http.Request) {
	var req share.ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.apiError(w, r, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}

	sh, err := app.Share.RequestShare(req, auth.UserFromContext(r.Context()))
	if err != nil {
		app.apiError(w, r, err)
		return
	}

	app.writeAPI(w, http.StatusCreated, sh)
}

func (app *App) getUploadOffset(w http.ResponseWriter, r *http.Request) {
	actor := auth.UserFromContext(r.Context())
	sh, err := app.Share.GetShare(r.PathValue("id"), actor)
	if err != nil {
		app.apiError(w, r, err)
		return
	}

	upload, err := app.Share.GetUploadState(sh, actor)
	if err != nil {
		app.apiError(w, r, err)
		return
	}

	offset := upload.Written(r.PathValue("fileID"))

	w.Header().Set("Upload-Offset", strconv.FormatInt(offset, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (app *App) addFilesToShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files []share.FileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.apiError(w, r, apperr.New(http.StatusBadRequest, err.Error()))
		return
	}

	newFiles, err := app.Share.UploadFiles(r.PathValue("id"), req.Files, auth.UserFromContext(r.Context()))
	if err != nil {
		app.apiError(w, r, err)
		return
	}

	app.writeAPI(w, http.StatusOK, newFiles)
}

func (app *App) streamFile(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		app.apiError(w, r, apperr.New(http.StatusBadRequest, "invalid Upload-Offset header"))
		return
	}

	bytesWritten, err := app.Share.StreamFile(r.PathValue("id"), r.PathValue("fileID"), offset, r.Body, auth.UserFromContext(r.Context()))
	if err != nil {
		var conflict *share.ChunkOffsetConflictError
		if errors.As(err, &conflict) {
			w.Header().Set("Upload-Offset", strconv.FormatInt(conflict.CurrentOffset, 10))
		}
		app.apiError(w, r, err)
		return
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(bytesWritten, 10))
	w.WriteHeader(http.StatusNoContent)
}

func (app *App) deleteShareFile(w http.ResponseWriter, r *http.Request) {
	if err := app.Share.DeleteFile(r.PathValue("id"), r.PathValue("fileID"), auth.UserFromContext(r.Context())); err != nil {
		app.apiError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
