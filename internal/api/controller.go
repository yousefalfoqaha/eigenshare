package api

import (
	"encoding/json"
	"net/http"

	"eigenshare/internal/middleware"
	"eigenshare/internal/session"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type Handler func(w http.ResponseWriter, r *http.Request) error

type Controller struct {
	Auth  *session.SessionService
	User  *user.UserService
	Share *share.ShareService
}

func NewController(authSvc *session.SessionService, userSvc *user.UserService, shareSvc *share.ShareService) *Controller {
	return &Controller{Auth: authSvc, User: userSvc, Share: shareSvc}
}

func (c *Controller) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /shares", c.handle(c.CreateShare))
	mux.Handle("POST /shares/{id}/files", c.handle(c.AddFilesToShare))
	mux.Handle("HEAD /shares/{id}/files/{fileID}/stream", c.handle(c.GetUploadOffset))
	mux.Handle("PUT /shares/{id}/files/{fileID}/stream", c.handle(c.StreamFile))
	mux.Handle("DELETE /shares/{id}/files/{fileID}", c.handle(c.DeleteShareFile))

	return middleware.RequireSession(writeUnauthorized)(mux)
}

func (c *Controller) handle(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			status, msg := middleware.MapError(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": msg})
		}
	}
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  http.StatusUnauthorized,
		"message": "unauthorized",
	})
}
