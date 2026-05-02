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
	router.HandleFunc("GET /shares/{id}/progress", MakeHandler(c.getProgress))
	router.HandleFunc("POST /shares", MakeHandler(c.createShare))
	router.HandleFunc("PUT /shares/{shareID}/files/{fileID}/stream", MakeHandler(c.streamFile))
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
			Name:         f.Name,
			ExpectedSize: f.ExpectedSize,
			Order:        f.Order,
		}
	}

	sh, err := c.shareService.RequestShare(params)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusCreated, sh)
}

func (c *ShareController) streamFile(w http.ResponseWriter, r *http.Request) error {
	shareID := r.PathValue("shareID")
	fileID := r.PathValue("fileID")

	offset, err := strconv.ParseInt(r.Header.Get("X-Write-Offset"), 10, 64)
	if err != nil {
		return api.NewError(http.StatusBadRequest, "invalid X-Write-Offset header")
	}

	if err := c.shareService.StreamFile(shareID, fileID, offset, r.Body); err != nil {
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

func (c *ShareController) getProgress(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	bytesWritten, err := c.shareService.GetShareProgress(id)
	if err != nil {
		return err
	}

	return JSON(w, http.StatusOK, map[string]int64{"bytesWritten": bytesWritten})
}

func (c *ShareController) download(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")

	dirPath, err := c.shareService.DownloadAll(id)
	if err != nil {
		return err
	}

	http.ServeFile(w, r, dirPath)
	return nil
}
