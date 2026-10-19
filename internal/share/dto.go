package share

type FileRequest struct {
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

type ShareRequest struct {
	Handle        string     `json:"handle"`
	ExpiryMinutes int        `json:"expiryMinutes"`
	MaxBytes      int64      `json:"maxBytes"`
	Visibility    Visibility `json:"visibility"`
	Password      string     `json:"password"`
}

type UploadLinkRequest struct {
	ExpiryMinutes int   `json:"expiryMinutes"`
	MaxBytes      int64 `json:"maxBytes"`
}
