package share

import (
	"fmt"
	"regexp"
)

type Share struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Status string `json:"status"`
	Files  []File `json:"files"`
}

var validShareHandle = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func (s *Share) setShareHandle(handle string) error {
	if handle == "" {
		return fmt.Errorf("share handle is required")
	}
	if !validShareHandle.MatchString(handle) {
		return fmt.Errorf("share handle may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}

	s.Handle = handle

	return nil
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

func (f *File) markChunkUploaded(index int) error {
	if index >= f.Chunks || index < 0 {
		return fmt.Errorf("chunk index %d out of bounds", index)
	}

	bitmaskIndex := index / 8
	bitOffset := index % 8
	mask := byte(1 << bitOffset)

	if f.Bitmask[bitmaskIndex]&mask != 0 {
		return fmt.Errorf("Chunk %d was already uploaded", index)
	}

	f.Bitmask[bitmaskIndex] |= mask

	return nil
}

func (f *File) rollbackChunk(index int) error {
	if index >= f.Chunks || index < 0 {
		return fmt.Errorf("chunk index %d out of bounds", index)
	}

	bitmaskIndex := index / 8
	bitOffset := index % 8
	mask := byte(1 << bitOffset)

	if f.Bitmask[bitmaskIndex]&mask == 0 {
		return nil
	}

	f.Bitmask[bitmaskIndex] &^= mask

	return nil
}
