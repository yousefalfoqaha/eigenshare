package share

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"time"

	"eigenshare/internal/apperr"
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
		return nil, apperr.New(http.StatusBadRequest, "share handle is required")
	}
	if !validShareHandle.MatchString(handle) {
		return nil, apperr.New(http.StatusBadRequest, "share handle may only contain letters, numbers, hyphens and underscores (3-64 chars)")
	}
	if expiryMinutes < 0 {
		return nil, apperr.New(http.StatusBadRequest, "expiry minutes must be non-negative")
	}
	if visibility == "" {
		visibility = VisibilityPrivate
	}
	if !visibility.Valid() {
		return nil, apperr.New(http.StatusBadRequest, "visibility must be public or private")
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
		return nil, apperr.New(http.StatusBadRequest, "file name is required")
	}
	if !validFileName.MatchString(name) {
		return nil, apperr.New(http.StatusBadRequest, "file name is invalid")
	}
	if expectedSize <= 0 {
		return nil, apperr.New(http.StatusBadRequest, "file size must be greater than 0")
	}
	return &File{
		Name:         name,
		ExpectedSize: expectedSize,
	}, nil
}

// UploadState is a share's live upload state: the bytes written to disk
// per file. The map is an internal detail — the file storage hands the
// state to the service, and consumers only ask it questions. The state
// is share-scoped: it answers for the files of the share it came from.
type UploadState struct {
	written map[string]int64
}

// Uploaded reports whether f's bytes are all on disk. The whole File is
// the minimal sufficient question — the rule needs the file's expected
// size, which the id alone does not carry.
func (u *UploadState) Uploaded(f File) bool {
	return u != nil && u.written[f.ID] >= f.ExpectedSize
}

// MarshalJSON presents the state as its bytes-written map — the
// uploader's inline seed ({"fileID": bytes}).
func (u *UploadState) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.written)
}
