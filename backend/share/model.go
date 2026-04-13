package share

type Share struct {
	ID    string `json:"id"`
	Files []File `json:"files"`
}

type ShareSummary struct {
	ID         string `json:"id"`
	TotalFiles int    `json:"totalFiles"`
	TotalSize  int    `json:"totalSize"`
}

type File struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int    `json:"size"`
}
