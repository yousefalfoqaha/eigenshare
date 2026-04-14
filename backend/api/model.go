package api

import "net/http"

type Error struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (e Error) Error() string {
	return e.Message
}

type controllerHandleFunc func(w http.ResponseWriter, r *http.Request) error
