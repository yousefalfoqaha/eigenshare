package share

type FileRequest struct {
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

type ShareRequest struct {
	Handle        string     `json:"handle"`
	ExpiryMinutes int        `json:"expiryMinutes"`
	Visibility    Visibility `json:"visibility"`
	Password      string     `json:"password"`
}

// ShareSummary is the read shape for a share in listing and edit views.
type ShareSummary struct {
	Share      *Share
	Upload     map[string]FileUploadState
	OwnerNames []string
}
