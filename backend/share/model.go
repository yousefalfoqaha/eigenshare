package share

import (
	"fmt"
	"regexp"
)

var (
	validShareHandle = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	validFileName    = regexp.MustCompile(`^[^/\\\x00]{1,255}$`)
)

type Share struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Status string `json:"status"`
	Files  []File `json:"files"`
}

func NewShare(id, handle string) (*Share, error) {
	if handle == "" {
		return nil, fmt.Errorf("share handle is required")
	}
	if !validShareHandle.MatchString(handle) {
		return nil, fmt.Errorf("share handle may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}

	return &Share{
		ID:     id,
		Handle: handle,
		Status: STATUS_PENDING,
	}, nil
}

type ShareSummary struct {
	ID         string `json:"id"`
	Handle     string `json:"handle"`
	Status     string `json:"status"`
	TotalFiles int    `json:"totalFiles"`
	TotalSize  int    `json:"totalSize"`
}

type File struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

func NewFile(name string, size int64, index int) (*File, error) {
	if name == "" {
		return nil, fmt.Errorf("file name is required")
	}
	if !validFileName.MatchString(name) {
		return nil, fmt.Errorf("file name is invalid")
	}
	if size <= 0 {
		return nil, fmt.Errorf("file size must be greater than 0")
	}
	if index < 0 {
		return nil, fmt.Errorf("file index must be greater than or equal to 0")
	}

	return &File{
		Index: index,
		Name:  name,
		Size:  size,
	}, nil
}
