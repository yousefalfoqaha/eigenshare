package share

import (
	"errors"
)

var (
	ErrInvalidHandle          = errors.New("invalid share handle")
	ErrInvalidFileName        = errors.New("invalid file name")
	ErrInvalidFileSize        = errors.New("invalid file size")
	ErrInvalidExpiry          = errors.New("invalid expiry")
	ErrInvalidVisibility      = errors.New("invalid visibility")
	ErrInvalidChunkOffset     = errors.New("invalid chunk offset")
	ErrInvalidMaxBytes        = errors.New("invalid max size")
	ErrShareNotFound          = errors.New("share not found")
	ErrFileNotFound           = errors.New("file not found")
	ErrUploadLinkNotFound     = errors.New("upload link not found")
	ErrHandleExists           = errors.New("handle already exists")
	ErrFileNotComplete        = errors.New("file is not complete")
	ErrChunkOffsetExceedsSize = errors.New("chunk offset exceeds declared file size")
	ErrShareVersionConflict   = errors.New("share was modified concurrently")
	ErrShareTrashed           = errors.New("share is trashed")
	ErrShareNotTrashed        = errors.New("share is not trashed")
	ErrLastOwner              = errors.New("cannot remove the last owner of a share")
	ErrOwnerExists            = errors.New("user is already an owner of this share")
	ErrUploadLinkLimit        = errors.New("upload link size limit reached")
	ErrShareLimit             = errors.New("share size limit reached")
	ErrUploadInProgress       = errors.New("an upload is already in progress for this file")
)
