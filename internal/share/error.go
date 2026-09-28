package share

import (
	"errors"
	"fmt"
)

var (
	// 400 — invalid input
	ErrInvalidRequest     = errors.New("invalid request")
	ErrInvalidHandle      = errors.New("invalid share handle")
	ErrInvalidFileName    = errors.New("invalid file name")
	ErrInvalidFileSize    = errors.New("invalid file size")
	ErrInvalidExpiry      = errors.New("invalid expiry")
	ErrInvalidVisibility  = errors.New("invalid visibility")
	ErrInvalidChunkOffset = errors.New("invalid chunk offset")

	// 404 — not found
	ErrShareNotFound = errors.New("share not found")
	ErrFileNotFound  = errors.New("file not found")

	// 409 — conflict
	ErrHandleExists           = errors.New("handle already exists")
	ErrFileNotComplete        = errors.New("file is not complete")
	ErrChunkOffsetExceedsSize = errors.New("chunk offset exceeds declared file size")
	ErrShareVersionConflict   = errors.New("share was modified concurrently")
	ErrShareTrashed           = errors.New("share is trashed")
	ErrShareNotTrashed        = errors.New("share is not trashed")
	ErrLastOwner              = errors.New("cannot remove the last owner of a share")
	ErrOwnerExists            = errors.New("user is already an owner of this share")

	// 423 — locked
	ErrUploadInProgress = errors.New("an upload is already in progress for this file")
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
