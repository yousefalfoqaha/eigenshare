package web

import (
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

func page(r *http.Request, title string) *Page {
	return &Page{Title: title, CurrentUser: auth.UserFromContext(r.Context())}
}

// Page is the data for the whole rendered page: the layout reads the chrome
// (title, banner, current user), and content templates receive the same
// Page, finding the handler's view under Content. Error and Content are
// written by the makeHTMLHandler wrapper only; handlers write Title and
// their view.
type Page struct {
	Title       string
	Error       string
	CurrentUser *user.User
	Content     any
}

func (p *Page) IsAdmin() bool {
	return p != nil && p.CurrentUser != nil && p.CurrentUser.IsAdmin
}

type SharesContent struct {
	Shares   []share.ShareSummary
	Progress map[string]int64
}

type EditShareContent struct {
	Share    *share.Share
	Progress map[string]int64
}

type ShareContent struct {
	Share *share.Share
}

type UsersContent struct {
	Users []*user.User
}

type UserFormContent struct {
	User *user.User
}
