package share

import "errors"

var (
	ErrHandleExists           = errors.New("handle already exists")
	ErrShareNotFound          = errors.New("share not found")
	ErrFileNotFound           = errors.New("file not found")
	ErrShareNotComplete       = errors.New("share is not complete")
	ErrChunkOffsetExceedsSize = errors.New("chunk offset exceeds declared file size")
	ErrInvalidChunkOffset     = errors.New("invalid chunk offset")
	ErrInvalidRequest         = errors.New("invalid request")
)
