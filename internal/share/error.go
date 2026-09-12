package share

import (
	"errors"
	"fmt"
)

var (
	ErrHandleExists           = errors.New("handle already exists")
	ErrShareNotFound          = errors.New("share not found")
	ErrFileNotFound           = errors.New("file not found")
	ErrShareNotComplete       = errors.New("share is not complete")
	ErrChunkOffsetExceedsSize = errors.New("chunk offset exceeds declared file size")
	ErrInvalidChunkOffset     = errors.New("invalid chunk offset")
	ErrInvalidRequest         = errors.New("invalid request")
	ErrShareVersionConflict   = errors.New("share was modified concurrently")
)

type ChunkOffsetConflictError struct {
	CurrentOffset int64
	ExpectedSize  int64
}

func (e *ChunkOffsetConflictError) Error() string {
	return fmt.Sprintf("invalid chunk offset; current offset is %d", e.CurrentOffset)
}

func (e *ChunkOffsetConflictError) Unwrap() error {
	return ErrInvalidChunkOffset
}
