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
	router.HandleFunc("POST /shares", api.MakeHandler(c.share))
}

func (c *Controller) share(w http.ResponseWriter, r *http.Request) error {
	reader, err := r.MultipartReader()
	if err != nil {
		return api.NewError(http.StatusInternalServerError, "Failed to get multipart reader", err)
	}

	share, err := c.shareService.share(reader)
	if err != nil {
		return err
	}

	return api.JSON(w, http.StatusCreated, share)
}
