package share

type FileRequest struct {
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

type ShareRequest struct {
	Handle        string `json:"handle"`
	ExpiryMinutes int    `json:"expiryMinutes"`
}
