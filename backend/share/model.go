package share

import "fmt"

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

func (f *File) createBitmask() error {
	if f.Size <= 0 {
		return fmt.Errorf("Invalid file size")
	}

	const CHUNK_SIZE_MB = 5
	var chunkSize int64 = 1024 * 1024 * CHUNK_SIZE_MB

	numChunks := (f.Size + chunkSize - 1) / chunkSize
	numBytes := (numChunks + 7) / 8

	f.Bitmask = make([]byte, numBytes)
	f.Chunks = int(numChunks)

	return nil
}
