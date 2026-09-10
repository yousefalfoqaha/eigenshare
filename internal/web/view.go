package web

import (
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func page(r *http.Request, title string) Page {
	return Page{Title: title, CurrentUser: auth.UserFromContext(r.Context())}
}

type Page struct {
	Title       string
	Error       string
	CurrentUser *user.User
}

type SharesView struct {
	Page
	Shares []share.ShareSummary
}

type ShareView struct {
	Page
	Share        *share.Share
	numFiles     int
	BytesWritten int64
	Complete     bool
}

type SharePublicView struct {
	Page
	Share *share.Share
}

type UsersView struct {
	Page
	Users []*user.User
}

type UserFormView struct {
	Page
	User *user.User
}
