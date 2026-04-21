package share

import "fmt"

type UploadState struct {
	Bitmask     []byte
	TotalChunks int
}

func (s *UploadState) NewUploadState(fileSize int64) error {
	if fileSize <= 0 {
		return fmt.Errorf("Invalid file size")
	}

	const CHUNK_SIZE_MB = 5
	var chunkSize int64 = 1024 * 1024 * CHUNK_SIZE_MB

	numChunks := (fileSize + chunkSize - 1) / chunkSize
	numBytes := (numChunks + 7) / 8

	s.Bitmask = make([]byte, numBytes)
	s.TotalChunks = int(numChunks)

	return nil
}

func (s *UploadState) MarkChunkUploaded(index int) error {
	if index >= s.TotalChunks || index < 0 {
		return fmt.Errorf("chunk index %d out of bounds", index)
	}

	bitmaskIndex := index / 8
	bitOffset := index % 8
	mask := byte(1 << bitOffset)

	if s.Bitmask[bitmaskIndex]&mask != 0 {
		return fmt.Errorf("Chunk %d was already uploaded", index)
	}

	s.Bitmask[bitmaskIndex] |= mask

	return nil
}
