package share

type Share struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Status string `json:"status"`
	Files  []File `json:"files"`
}

type ShareSummary struct {
	ID         string `json:"id"`
	Handle     string `json:"handle"`
	Status     string `json:"status"`
	TotalFiles int    `json:"totalFiles"`
	TotalSize  int    `json:"totalSize"`
}

type File struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Chunks  int    `json:"chunks"`
	Bitmask []byte `json:"bitmask"`
}
