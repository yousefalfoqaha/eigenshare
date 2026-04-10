package share

import (
	"encoding/json"
	"net/http"
)

type Controller struct {
	shareRepository *Repository
}

func NewController(shareRepository *Repository) *Controller {
	return &Controller{shareRepository: shareRepository}
}

func (c *Controller) RegisterHandlers(router *http.ServeMux) {
	router.HandleFunc("GET /shares", makeHandler(c.getAllSharesHandler))
}

func (c *Controller) getAllSharesHandler(w http.ResponseWriter, r *http.Request) error {
	shares, err := c.shareRepository.GetAll()
	if err != nil {
		return ApiError{Status: http.StatusInternalServerError, Message: "Something went wrong."}
	}

	return JSON(w, shares, http.StatusOK)
}

func JSON(w http.ResponseWriter, v any, status int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

type ApiError struct {
	Status  int
	Message string
}

func (e ApiError) Error() string {
	return e.Message
}

type controllerHandleFunc func(w http.ResponseWriter, r *http.Request) error

func makeHandler(h controllerHandleFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			enc.Encode(err)
		}
	}
}
