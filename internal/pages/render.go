package pages

import (
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/middleware"
	"eigenshare/internal/user"
)

type pageData struct {
	Banner      string
	CurrentUser *user.User
	Content     any
}

// render executes a page template. domainErr, when present, is mapped to a
// user-safe message and shown as the banner; on success the caller redirects
// (PRG) and returns nil.
func (c *Controller) render(w http.ResponseWriter, r *http.Request, tmpl string, content any, domainErr error) error {
	banner := ""
	if domainErr != nil {
		_, banner = middleware.MapError(domainErr)
	}
	if banner == "" && r.URL.Query().Has("err") {
		banner = r.URL.Query().Get("err")
	}

	pg := pageData{
		Banner:      banner,
		CurrentUser: auth.UserFromContext(r.Context()),
		Content:     content,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return c.templates[tmpl].ExecuteTemplate(w, "layout", &pg)
}
