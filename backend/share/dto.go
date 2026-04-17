package share

type fileRequest struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

type shareRequest struct {
	Handle string        `json:"handle"`
	Files  []fileRequest `json:"files"`
}
