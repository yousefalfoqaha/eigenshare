package api

import (
	"encoding/json"
	"log"
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

	mux.Handle("POST /shares/{id}/files", handle(c.AddFilesToShare))
	mux.Handle("HEAD /shares/{id}/files/{fileID}/stream", handle(c.GetUploadOffset))
	mux.Handle("PUT /shares/{id}/files/{fileID}/stream", handle(c.StreamFile))
	mux.Handle("DELETE /shares/{id}/files/{fileID}", handle(c.DeleteShareFile))

	return middleware.RequireSession(writeUnauthorized)(mux)
}

// handle runs the handler and writes a JSON error response on failure.
func handle(h func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, r, err)
		}
	}
}

// writeError maps the error, logs 5xx failures and writes the JSON error response.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := middleware.MapHTTPError(err)
	if status >= http.StatusInternalServerError {
		log.Printf("INTERNAL ERROR: %v", err)
	}

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
