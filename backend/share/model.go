package share

type Share struct {
	ID    string `json:"id"`
	Files []File `json:"files"`
}

type File struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int    `json:"size"`
}
