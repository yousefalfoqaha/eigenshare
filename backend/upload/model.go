package upload

import "time"

type FileUploadStatus struct {
	FileIndex      int `json:"fileIndex"`
	ExpectedChunks int `json:"totalChunks"`
	MarkedChunks   int `json:"chunksUploaded"`
}

type UploadProgress struct {
	Files          []FileUploadStatus `json:"files"`
	LastActivityAt time.Time          `json:"lastActivityAt"`
}
