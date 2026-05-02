package handler

type fileRequest struct {
	Order        int    `json:"order"`
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

type shareRequest struct {
	Handle string        `json:"handle"`
	Files  []fileRequest `json:"files"`
}
