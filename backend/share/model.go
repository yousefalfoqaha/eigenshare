package share

import (
	"eigenshare/backend/timestamp"
	"fmt"
	"regexp"
	"time"
)

var (
	validShareHandle = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	validFileName    = regexp.MustCompile(`^[^/\\\x00]{1,255}$`)
)

type Share struct {
	ID        string               `json:"id"`
	Handle    string               `json:"handle"`
	Files     []File               `json:"files"`
	CreatedAt timestamp.Timestamp  `json:"createdAt"`
	ExpiresAt *timestamp.Timestamp `json:"expiresAt"`
}

func NewShare(id, handle string, expiryMinutes int) (*Share, error) {
	if handle == "" {
		return nil, fmt.Errorf("share handle is required")
	}
	if !validShareHandle.MatchString(handle) {
		return nil, fmt.Errorf("share handle may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}
	if expiryMinutes < 0 {
		return nil, fmt.Errorf("expiry minutes must be non-negative")
	}

	createdAt := timestamp.Now()
	s := &Share{
		ID:        id,
		Handle:    handle,
		CreatedAt: createdAt,
	}

	if expiryMinutes > 0 {
		expiresAt := timestamp.Timestamp(time.Time(createdAt).Add(time.Duration(expiryMinutes) * time.Minute))
		s.ExpiresAt = &expiresAt
	}

	return s, nil
}

type ShareSummary struct {
	ID         string `json:"id"`
	Handle     string `json:"handle"`
	TotalFiles int    `json:"totalFiles"`
	TotalSize  int    `json:"totalSize"`
}

type File struct {
	ID           string `json:"id"`
	Order        int    `json:"order"`
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

func NewFile(name string, expectedSize int64, order int) (*File, error) {
	if name == "" {
		return nil, fmt.Errorf("file name is required")
	}
	if !validFileName.MatchString(name) {
		return nil, fmt.Errorf("file name is invalid")
	}
	if expectedSize <= 0 {
		return nil, fmt.Errorf("file size must be greater than 0")
	}
	if order < 0 {
		return nil, fmt.Errorf("file order must be greater than or equal to 0")
	}

	return &File{
		Order:        order,
		Name:         name,
		ExpectedSize: expectedSize,
	}, nil
}
