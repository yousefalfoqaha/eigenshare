package share

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"time"

	"eigenshare/internal/timestamp"
	"eigenshare/internal/user"
)

var (
	validShareHandle = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,64}$`)
	validFileName    = regexp.MustCompile(`^[^/\\\x00]{1,255}$`)
)

type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

func (v Visibility) Valid() bool {
	return v == VisibilityPublic || v == VisibilityPrivate
}

type Share struct {
	ID         string               `json:"id"`
	Handle     string               `json:"handle"`
	Visibility Visibility           `json:"visibility"`
	Owners     []string             `json:"owners"`
	Files      []File               `json:"files"`
	CreatedAt  timestamp.Timestamp  `json:"createdAt"`
	ExpiresAt  *timestamp.Timestamp `json:"expiresAt"`
	TrashedAt  *timestamp.Timestamp `json:"trashedAt"`
	Version    int64                `json:"-"`
}

func NewShare(id, handle string, expiryMinutes int, visibility Visibility, creatorID string) (*Share, error) {
	if handle == "" {
		return nil, fmt.Errorf("share handle is required: %w", ErrInvalidHandle)
	}
	if !validShareHandle.MatchString(handle) {
		return nil, fmt.Errorf("share handle may only contain letters, numbers, hyphens and underscores (3-64 chars): %w", ErrInvalidHandle)
	}
	if expiryMinutes < 0 {
		return nil, fmt.Errorf("expiry minutes must be non-negative: %w", ErrInvalidExpiry)
	}
	if visibility == "" {
		visibility = VisibilityPrivate
	}
	if !visibility.Valid() {
		return nil, fmt.Errorf("visibility must be public or private: %w", ErrInvalidVisibility)
	}

	createdAt := timestamp.Now()
	s := &Share{
		ID:         id,
		Handle:     handle,
		Visibility: visibility,
		Owners:     []string{},
		CreatedAt:  createdAt,
	}

	if creatorID != "" {
		s.Owners = append(s.Owners, creatorID)
	}

	if expiryMinutes > 0 {
		expiresAt := timestamp.Timestamp(time.Time(createdAt).Add(time.Duration(expiryMinutes) * time.Minute))
		s.ExpiresAt = &expiresAt
	}

	return s, nil
}

func (s *Share) Trashed() bool {
	return s.TrashedAt != nil
}

func (s *Share) IsOwner(u *user.User) bool {
	return u != nil && slices.Contains(s.Owners, u.ID)
}

func (s *Share) CanManage(u *user.User) bool {
	return u != nil && (u.IsAdmin || s.IsOwner(u))
}

func (s *Share) CanView(u *user.User) bool {
	return s.Visibility == VisibilityPublic || s.CanManage(u)
}

type File struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ExpectedSize int64  `json:"expectedSize"`
}

func NewFile(name string, expectedSize int64) (*File, error) {
	if name == "" {
		return nil, fmt.Errorf("file name is required: %w", ErrInvalidFileName)
	}
	if !validFileName.MatchString(name) {
		return nil, fmt.Errorf("file name is invalid: %w", ErrInvalidFileName)
	}
	if expectedSize <= 0 {
		return nil, fmt.Errorf("file size must be greater than 0: %w", ErrInvalidFileSize)
	}
	return &File{
		Name:         name,
		ExpectedSize: expectedSize,
	}, nil
}

type UploadState struct {
	written map[string]int64
}

func (u *UploadState) Uploaded(f File) bool {
	return u != nil && u.written[f.ID] >= f.ExpectedSize
}

func (u *UploadState) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.written)
}

func (u *UploadState) Written(fileID string) int64 {
	if u == nil {
		return 0
	}
	return u.written[fileID]
}
