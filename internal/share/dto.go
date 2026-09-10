package share

type FileRequest struct {
	Order        int    `json:"order"`
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

type ShareRequest struct {
	Handle        string        `json:"handle"`
	ExpiryMinutes int           `json:"expiryMinutes"`
	Files         []FileRequest `json:"files"`
}
