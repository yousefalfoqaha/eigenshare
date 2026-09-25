package share

import (
	"fmt"
	"net/http"

	"eigenshare/internal/apperr"
)

var (
	ErrHandleExists           = apperr.New(http.StatusConflict, "handle already exists")
	ErrShareNotFound          = apperr.New(http.StatusNotFound, "share not found")
	ErrFileNotFound           = apperr.New(http.StatusNotFound, "file not found")
	ErrFileNotComplete        = apperr.New(http.StatusConflict, "file is not complete")
	ErrChunkOffsetExceedsSize = apperr.New(http.StatusConflict, "chunk offset exceeds declared file size")
	ErrInvalidChunkOffset     = apperr.New(http.StatusBadRequest, "invalid chunk offset")
	ErrInvalidRequest         = apperr.New(http.StatusBadRequest, "invalid request")
	ErrShareVersionConflict   = apperr.New(http.StatusConflict, "share was modified concurrently")
	ErrShareTrashed           = apperr.New(http.StatusConflict, "share is trashed")
	ErrShareNotTrashed        = apperr.New(http.StatusConflict, "share is not trashed")
	ErrLastOwner              = apperr.New(http.StatusConflict, "cannot remove the last owner of a share")
	ErrOwnerExists            = apperr.New(http.StatusConflict, "user is already an owner of this share")
	ErrUploadInProgress       = apperr.New(http.StatusConflict, "an upload is already in progress for this file")
)

type ChunkOffsetConflictError struct {
	CurrentOffset int64
	ExpectedSize  int64
}

func (e *ChunkOffsetConflictError) Error() string {
	return fmt.Sprintf("invalid chunk offset; current offset is %d", e.CurrentOffset)
}

func (e *ChunkOffsetConflictError) HTTPStatus() int {
	return http.StatusConflict
}

func (e *ChunkOffsetConflictError) Unwrap() error {
	return ErrInvalidChunkOffset
}
