package share

import (
	"eigenshare/backend/api"
	"encoding/json"
	"net/http"
	"strconv"
)

type Controller struct {
	ShareService *Service
}

func (c *Controller) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /shares", api.MakeHandler(c.getShares))
	router.HandleFunc("GET /shares/{id}", api.MakeHandler(c.getShare))
	router.HandleFunc("POST /shares", api.MakeHandler(c.createShare))
}

func (c *Controller) createShare(w http.ResponseWriter, r *http.Request) error {
	var shareReq shareRequest
	err := json.NewDecoder(r.Body).Decode(&shareReq)
	if err != nil {
		return err
	}

	share, err := c.ShareService.requestShare(shareReq)
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusCreated, share)
}

func (c *Controller) uploadChunk(w http.ResponseWriter, r *http.Request) error {
	shareId := r.URL.Query().Get("shareId")

	fileIndex, err := strconv.Atoi(r.Header.Get("X-File-Index"))
	if err != nil {
		return err
	}

	chunkIndex, err := strconv.Atoi(r.Header.Get("X-Chunk-Index"))
	if err != nil {
		return err
	}

	err = c.ShareService.uploadChunk(shareId, fileIndex, chunkIndex, r.Body)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)

	return nil
}

func (c *Controller) getShares(w http.ResponseWriter, _ *http.Request) error {
	shares, err := c.ShareService.getShares()
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusOK, shares)
}

func (c *Controller) getShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	share, err := c.ShareService.getShare(id)
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusOK, share)
}
