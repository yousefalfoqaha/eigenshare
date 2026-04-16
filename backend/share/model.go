package share

type Share struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Status string `json:"status"`
	Files  []File `json:"files"`
}

type ShareSummary struct {
	ID         string `json:"id"`
	TotalFiles int    `json:"totalFiles"`
	TotalSize  int    `json:"totalSize"`
}

type File struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int    `json:"size"`
	Bitmask []byte `json:"bitmask"`
}

type fileRequest struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

type shareRequest struct {
	Handle string        `json:"handle"`
	Files  []fileRequest `json:"files"`
}
