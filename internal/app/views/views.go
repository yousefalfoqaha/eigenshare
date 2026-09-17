package views

import "net/http"

type View interface {
	Load(r *http.Request) error
	Title() string
}
