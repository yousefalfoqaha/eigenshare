package apperr

type Error struct {
	Message    string
	StatusCode int
}

func (e Error) Error() string   { return e.Message }
func (e Error) HTTPStatus() int { return e.StatusCode }

func New(statusCode int, message string) Error {
	return Error{Message: message, StatusCode: statusCode}
}
