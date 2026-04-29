package upload

type FileUploadStatus struct {
	FileIndex      int `json:"fileIndex"`
	TotalChunks    int `json:"totalChunks"`
	ChunksUploaded int `json:"chunksUploaded"`
}

type UploadStatus struct {
	Files []FileUploadStatus `json:"files"`
}
