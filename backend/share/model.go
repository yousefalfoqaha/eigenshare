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

func (s *Share) setHandle(handle string) error {
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
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

func (f *File) setName(name string) error {
	if name == "" {
		return fmt.Errorf("file name is required")
	}
	if !validFileName.MatchString(name) {
		return fmt.Errorf("file name is invalid")
	}

	f.Name = name

	return nil
}

func (f *File) setSize(size int64) error {
	if size <= 0 {
		return fmt.Errorf("file size must be greater than 0")
	}

	f.Size = size

	return nil
}

func (f *File) setIndex(index int) error {
	if index < 0 {
		return fmt.Errorf("file index must be greater than or equal to 0")
	}

	f.Index = index

	return nil
}
