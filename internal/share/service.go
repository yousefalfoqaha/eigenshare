package share

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"eigenshare/internal/security"
	"eigenshare/internal/timestamp"
	"eigenshare/internal/user"

	"github.com/google/uuid"
)

type ShareService struct {
	repo  *ShareRepository
	store *FileStorage
	users *user.UserService
}

func NewShareService(repo *ShareRepository, fs *FileStorage, users *user.UserService) *ShareService {
	return &ShareService{
		repo:  repo,
		store: fs,
		users: users,
	}
}

// StreamFile writes an upload chunk for a file on the share.
func (s *ShareService) StreamFile(sh *Share, fileID string, offset int64, stream io.Reader) (int64, error) {
	if sh.Trashed() {
		return 0, ErrShareTrashed
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return 0, ErrFileNotFound
	}

	if offset >= file.ExpectedSize {
		return 0, ErrChunkOffsetExceedsSize
	}

	bytesWritten, err := s.store.WriteFileStream(sh.ID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		return bytesWritten, err
	}

	return bytesWritten, nil
}

func (s *ShareService) RequestShare(req ShareRequest, u *user.User) (*Share, error) {
	var creatorID string
	if u != nil {
		creatorID = u.ID
	}
	sh, err := NewShare("", req.Handle, req.ExpiryMinutes, req.Visibility, creatorID)
	if err != nil {
		return nil, err
	}

	sh.PasswordHash, err = hashSharePassword(req.Password)
	if err != nil {
		return nil, err
	}

	return s.repo.Save(sh)
}

// UploadFiles registers new file records on the share. When req is non-nil
// the files are attributed to that upload request, counting against its
// size quota.
func (s *ShareService) UploadFiles(sh *Share, fileReqs []FileRequest, req *UploadRequest) ([]File, error) {
	if sh.Trashed() {
		return nil, ErrShareTrashed
	}

	if req != nil {
		var total int64
		for _, f := range fileReqs {
			total += f.ExpectedSize
		}
		if req.MaxBytes > 0 && req.BytesUsed+total > req.MaxBytes {
			return nil, ErrUploadRequestLimit
		}
	}

	var newFiles []File

	for _, fileReq := range fileReqs {
		file, err := NewFile(fileReq.Name, fileReq.ExpectedSize)
		if err != nil {
			return nil, err
		}
		file.ID = uuid.New().String()
		sh.Files = append(sh.Files, *file)
		if req != nil {
			req.FileIDs = append(req.FileIDs, file.ID)
			req.BytesUsed += file.ExpectedSize
		}
		newFiles = append(newFiles, *file)
	}

	if _, err := s.repo.Save(sh); err != nil {
		return nil, err
	}

	return newFiles, nil
}

// CreateUploadRequest adds a new upload request to the share.
func (s *ShareService) CreateUploadRequest(sh *Share, req UploadRequestRequest) (*UploadRequest, error) {
	if sh.Trashed() {
		return nil, ErrShareTrashed
	}

	ur, err := NewUploadRequest(req)
	if err != nil {
		return nil, err
	}

	sh.UploadRequests = append(sh.UploadRequests, *ur)

	if _, err := s.repo.Save(sh); err != nil {
		return nil, err
	}

	return ur, nil
}

// RevokeUploadRequest deletes the upload request from the share.
func (s *ShareService) RevokeUploadRequest(sh *Share, id string) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	idx := -1
	for i := range sh.UploadRequests {
		if sh.UploadRequests[i].ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrUploadRequestNotFound
	}

	sh.UploadRequests = append(sh.UploadRequests[:idx], sh.UploadRequests[idx+1:]...)

	_, err := s.repo.Save(sh)

	return err
}

// GetShareByHandle fetches a share by its handle. Access policy is not
// enforced here; route middleware decides what the caller may do with
// the share.
func (s *ShareService) GetShareByHandle(handle string) (*Share, error) {
	return s.repo.GetByHandle(handle)
}

// GetShare fetches a share by ID without enforcing access policy; see
// GetShareByHandle.
func (s *ShareService) GetShare(id string) (*Share, error) {
	return s.repo.Get(id)
}

func (s *ShareService) Delete(u *user.User, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		sh, err := s.repo.Get(id)
		if err != nil {
			return err
		}
		if !sh.CanManage(u) {
			return ErrShareNotFound
		}
	}

	return s.repo.Delete(ids...)
}

func (s *ShareService) GetShares(u *user.User) ([]*Share, error) {
	shares, err := s.repo.GetAll()
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(shares, func(sh *Share) bool {
		return !sh.CanView(u)
	}), nil
}

func (s *ShareService) GetTrashedShares(u *user.User) ([]*Share, error) {
	shares, err := s.repo.GetAllTrashed()
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(shares, func(sh *Share) bool {
		return !sh.CanManage(u)
	}), nil
}

// GetFileUploadState reports the on-disk upload progress of a file.
func (s *ShareService) GetFileUploadState(sh *Share, fileID string) (FileUploadState, error) {
	return s.store.FileUploadState(sh.ID, fileID)
}

// GetShareUploadState reports the on-disk upload progress of every file
// on the share.
func (s *ShareService) GetShareUploadState(sh *Share) (map[string]FileUploadState, error) {
	states := make(map[string]FileUploadState, len(sh.Files))
	for _, f := range sh.Files {
		st, err := s.store.FileUploadState(sh.ID, f.ID)
		if err != nil {
			return nil, err
		}
		states[f.ID] = st
	}

	return states, nil
}

// DownloadAll streams the share as a zip archive.
func (s *ShareService) DownloadAll(sh *Share, w io.Writer) error {
	return s.store.CreateShareZipStream(*sh, w)
}

// DownloadFile returns a completed file's metadata and path.
func (s *ShareService) DownloadFile(sh *Share, fileID string) (*File, string, error) {
	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return nil, "", ErrFileNotFound
	}

	state, err := s.store.FileUploadState(sh.ID, file.ID)
	if err != nil {
		return nil, "", err
	}

	if state.Written < file.ExpectedSize {
		return nil, "", ErrFileNotComplete
	}

	path, err := s.store.ShareFilePath(sh.ID, file.ID)
	if err != nil {
		return nil, "", err
	}

	return file, path, nil
}

// DeleteFile removes a file record from the share. Files registered through
// an upload request are detached from it, but their declared size is not
// returned to the request's quota.
func (s *ShareService) DeleteFile(sh *Share, fileID string) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	var removed *File
	for i := range sh.Files {
		if sh.Files[i].ID == fileID {
			f := sh.Files[i]
			removed = &f
			sh.Files = append(sh.Files[:i], sh.Files[i+1:]...)
			break
		}
	}
	if removed == nil {
		return ErrFileNotFound
	}

	for i := range sh.UploadRequests {
		req := &sh.UploadRequests[i]
		if !slices.Contains(req.FileIDs, removed.ID) {
			continue
		}
		idx := slices.Index(req.FileIDs, removed.ID)
		req.FileIDs = slices.Delete(req.FileIDs, idx, idx+1)
	}

	_, err := s.repo.Save(sh)

	return err
}

// TrashShare moves the share to the trash.
func (s *ShareService) TrashShare(sh *Share) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	now := timestamp.Now()
	sh.TrashedAt = &now

	_, err := s.repo.Save(sh)

	return err
}

// RecoverShare restores the share from the trash.
func (s *ShareService) RecoverShare(sh *Share) error {
	if !sh.Trashed() {
		return ErrShareNotTrashed
	}

	sh.TrashedAt = nil

	_, err := s.repo.Save(sh)

	return err
}

// AddOwner adds a user as an owner of the share.
func (s *ShareService) AddOwner(sh *Share, newOwnerID string) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	if _, err := s.users.GetUser(newOwnerID); err != nil {
		return err
	}

	if slices.Contains(sh.Owners, newOwnerID) {
		return ErrOwnerExists
	}

	sh.Owners = append(sh.Owners, newOwnerID)

	_, err := s.repo.Save(sh)

	return err
}

// RemoveOwner removes a user from the share's owners.
func (s *ShareService) RemoveOwner(sh *Share, ownerID string) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	idx := slices.Index(sh.Owners, ownerID)
	if idx == -1 {
		return ErrShareNotFound
	}
	if len(sh.Owners) == 1 {
		return ErrLastOwner
	}

	sh.Owners = slices.Delete(sh.Owners, idx, idx+1)

	_, err := s.repo.Save(sh)

	return err
}

// SetVisibility updates the share's visibility.
func (s *ShareService) SetVisibility(sh *Share, visibility Visibility) error {
	if !visibility.Valid() {
		return fmt.Errorf("visibility must be public or private: %w", ErrInvalidVisibility)
	}

	if sh.Trashed() {
		return ErrShareTrashed
	}

	if sh.Visibility == visibility {
		return nil
	}

	sh.Visibility = visibility

	_, err := s.repo.Save(sh)

	return err
}

// SetPassword sets, changes, or removes the share's password.
func (s *ShareService) SetPassword(sh *Share, raw string, remove bool) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	switch {
	case remove:
		sh.PasswordHash = ""
	case strings.TrimSpace(raw) != "":
		hash, err := hashSharePassword(raw)
		if err != nil {
			return err
		}
		sh.PasswordHash = hash
	}

	_, err := s.repo.Save(sh)

	return err
}

func hashSharePassword(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if len(raw) < 3 {
		return "", fmt.Errorf("password must be at least 3 characters: %w", security.ErrInvalidPassword)
	}
	if len(raw) > 72 {
		return "", fmt.Errorf("password must be at most 72 characters: %w", security.ErrInvalidPassword)
	}
	return security.HashPassword(raw)
}
