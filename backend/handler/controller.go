package handler

import (
	"eigenshare/backend/api"
	"eigenshare/backend/service"
	"encoding/json"
	"net/http"
	"strconv"
)

type ShareController struct {
	shareService *service.ShareService
}

func NewShareController(svc *service.ShareService) *ShareController {
	return &ShareController{shareService: svc}
}

func (c *ShareController) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /shares", MakeHandler(c.getShares))
	router.HandleFunc("GET /shares/{id}", MakeHandler(c.getShare))
	router.HandleFunc("GET /shares/{id}/download", MakeHandler(c.download))
	router.HandleFunc("GET /shares/{id}/upload-status", MakeHandler(c.getUploadStatus))
	router.HandleFunc("POST /shares", MakeHandler(c.createShare))
	router.HandleFunc("POST /shares/{id}/upload", MakeHandler(c.uploadChunk))
	router.HandleFunc("POST /shares/{id}/resume", MakeHandler(c.resumeShare))
}

func (c *ShareController) createShare(w http.ResponseWriter, r *http.Request) error {
	var shareReq shareRequest
	err := json.NewDecoder(r.Body).Decode(&shareReq)
	if err != nil {
		return err
	}

	params := service.CreateShareParams{
		Handle: shareReq.Handle,
		Files:  make([]service.CreateFileParams, len(shareReq.Files)),
	}
	for i, f := range shareReq.Files {
		params.Files[i] = service.CreateFileParams{
			Name:  f.Name,
			Size:  f.Size,
			Index: f.Index,
		}
	}

	sh, err := c.shareService.RequestShare(params)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusCreated, sh)
}

func (c *ShareController) uploadChunk(w http.ResponseWriter, r *http.Request) error {
	shareID := r.PathValue("id")

	fileIndex, err := strconv.Atoi(r.URL.Query().Get("fileIndex"))
	if err != nil {
		return api.NewError(http.StatusBadRequest, "invalid fileIndex")
	}

	chunkIndex, err := strconv.Atoi(r.URL.Query().Get("chunkIndex"))
	if err != nil {
		return api.NewError(http.StatusBadRequest, "invalid chunkIndex")
	}

	if err := c.shareService.UploadChunk(shareID, fileIndex, chunkIndex, r.Body); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (c *ShareController) getShares(w http.ResponseWriter, _ *http.Request) error {
	shares, err := c.shareService.GetShares()
	if err != nil {
		return err
	}

	return JSON(w, http.StatusOK, shares)
}

func (c *ShareController) getShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	sh, err := c.shareService.GetShare(id)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusOK, sh)
}

func (c *ShareController) getUploadStatus(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	status, err := c.shareService.GetUploadStatus(id)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusOK, status)
}

func (c *ShareController) download(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")

	fileIndex, err := strconv.Atoi(r.URL.Query().Get("fileIndex"))
	if err != nil {
		return err
	}

	filePath, err := c.shareService.Download(id, fileIndex)
	if err != nil {
		return err
	}

	http.ServeFile(w, r, filePath)
	return nil
}

func (c *ShareController) resumeShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	sh, err := c.shareService.ResumeShare(id)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusOK, sh)
}
