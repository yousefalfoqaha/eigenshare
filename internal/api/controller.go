package api

import (
	"encoding/json"
	"net/http"

	"eigenshare/internal/middleware"
	"eigenshare/internal/session"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type Controller struct {
	authSvc  *session.SessionService
	userSvc  *user.UserService
	shareSvc *share.ShareService
}

func NewController(authSvc *session.SessionService, userSvc *user.UserService, shareSvc *share.ShareService) *Controller {
	return &Controller{authSvc: authSvc, userSvc: userSvc, shareSvc: shareSvc}
}

func (c *Controller) Routes() http.Handler {
	mux := http.NewServeMux()

	handleErr := middleware.WithErrorHandler(writeError)

	mux.Handle("POST /shares", handleErr(c.CreateShare))
	mux.Handle("POST /shares/{id}/files", handleErr(c.AddFilesToShare))
	mux.Handle("HEAD /shares/{id}/files/{fileID}/stream", handleErr(c.GetUploadOffset))
	mux.Handle("PUT /shares/{id}/files/{fileID}/stream", handleErr(c.StreamFile))
	mux.Handle("DELETE /shares/{id}/files/{fileID}", handleErr(c.DeleteShareFile))

	return middleware.RequireSession(writeUnauthorized)(mux)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": msg})
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  http.StatusUnauthorized,
		"message": "unauthorized",
	})
}
