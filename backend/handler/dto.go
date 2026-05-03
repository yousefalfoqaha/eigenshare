package handler

type fileRequest struct {
	Order        int    `json:"order"`
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

type shareRequest struct {
	Handle        string        `json:"handle"`
	ExpiryMinutes int           `json:"expiryMinutes"`
	Files         []fileRequest `json:"files"`
}
