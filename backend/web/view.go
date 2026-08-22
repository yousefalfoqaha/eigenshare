package web

import (
	"eigenshare/backend/share"
	"eigenshare/backend/user"
)

type Error struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (e Error) Error() string {
	return e.Message
}

func NewError(status int, message string) Error {
	return Error{Status: status, Message: message}
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
