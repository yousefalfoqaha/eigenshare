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
	router.HandleFunc("POST /shares", api.MakeHandler(c.createShare))
	router.HandleFunc("GET /shares/{id}", api.MakeHandler(c.getShare))
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

func (c *Controller) getShare(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	share, err := c.shareService.getShare(id)
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusCreated, share)
}
