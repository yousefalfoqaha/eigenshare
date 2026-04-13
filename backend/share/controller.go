package share

import (
	"net/http"

	"eigenshare/backend/api"
)

type Controller struct {
	shareService *Service
}

func NewController(shareService *Service) *Controller {
	return &Controller{shareService}
}

func (c *Controller) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /shares", api.MakeHandler(c.getShares))
	router.HandleFunc("GET /shares/{id}", api.MakeHandler(c.getShare))
	router.HandleFunc("POST /shares", api.MakeHandler(c.createShare))
}

func (c *Controller) createShare(w http.ResponseWriter, r *http.Request) error {
	reader, err := r.MultipartReader()
	if err != nil {
		return err
	}

	share, err := c.shareService.share(reader)
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusCreated, share)
}

func (c *Controller) getShares(w http.ResponseWriter, _ *http.Request) error {
	shares, err := c.shareService.getShares()
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusOK, shares)
}

func (c *Controller) getShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	share, err := c.shareService.getShare(id)
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusCreated, share)
}
