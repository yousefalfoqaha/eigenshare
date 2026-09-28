package api

import (
	"encoding/json"
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/middleware"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

type Handler func(w http.ResponseWriter, r *http.Request) error

type Controller struct {
	Auth  *auth.AuthService
	User  *user.UserService
	Share *share.ShareService
}

func New(authSvc *auth.AuthService, userSvc *user.UserService, shareSvc *share.ShareService) *Controller {
	return &Controller{Auth: authSvc, User: userSvc, Share: shareSvc}
}

func (c *Controller) Wrap(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			status, msg := middleware.MapError(err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": msg})
		}
	}
}

func (c *Controller) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(middleware.SessionCookieName)
		if err != nil || cookie.Value == "" {
			writeUnauthorized(w)
			return
		}
		u, err := c.Auth.ValidateSession(cookie.Value)
		if err != nil {
			middleware.ClearSessionCookie(w)
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  http.StatusUnauthorized,
		"message": "unauthorized",
	})
}
