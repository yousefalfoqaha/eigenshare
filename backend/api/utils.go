package api

func NewError(status int, message string) Error {
	return Error{Status: status, Message: message}
}
