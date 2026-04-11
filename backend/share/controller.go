package share

import (
	"net/http"

	"eigenshare/backend/api"
)

type Controller struct {
	shareRepository *Repository
}

func NewController(shareRepository *Repository) *Controller {
	return &Controller{shareRepository}
}

func (c *Controller) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /shares", api.MakeHandler(c.getAllSharesHandler))
}

func (c *Controller) getAllSharesHandler(w http.ResponseWriter, r *http.Request) error {
	shares, err := c.shareRepository.GetAll()
	if err != nil {
		return api.Error{Status: http.StatusInternalServerError, Message: "Something went wrong."}
	}

	return api.JSON(w, http.StatusOK, shares)
}
