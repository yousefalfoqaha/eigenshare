package share

type Share struct {
	ID    string   `json:"id"`
	Files []string `json:"files"`
}
