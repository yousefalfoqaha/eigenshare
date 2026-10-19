package share

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"eigenshare/internal/security"
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
	sh, err := NewShare("", req.Handle, req.ExpiryMinutes, req.MaxBytes, req.Visibility, creatorID)
	if err != nil {
		return nil, err
	}

	sh.PasswordHash, err = hashSharePassword(req.Password)
	if err != nil {
		return nil, err
	}

	return s.repo.Save(sh)
}

func (s *ShareService) UploadFiles(sh *Share, fileReqs []FileRequest, link *UploadLink) ([]File, error) {
	if sh.Trashed() {
		return nil, ErrShareTrashed
	}

	if link != nil {
		var total int64
		for _, f := range fileReqs {
			total += f.ExpectedSize
		}
		if link.MaxBytes > 0 && link.BytesUsed+total > link.MaxBytes {
			return nil, ErrUploadLinkLimit
		}
	}

	var newTotal int64
	for _, f := range fileReqs {
		newTotal += f.ExpectedSize
	}
	if sh.MaxBytes > 0 && newTotal > sh.MaxBytes-sh.UploadedSize() {
		return nil, ErrShareLimit
	}

	var newFiles []File

	for _, fileReq := range fileReqs {
		file, err := NewFile(fileReq.Name, fileReq.ExpectedSize)
		if err != nil {
			return nil, err
		}
		file.ID = uuid.New().String()
		sh.Files = append(sh.Files, *file)
		if link != nil {
			link.BytesUsed += file.ExpectedSize
		}
		newFiles = append(newFiles, *file)
	}

	if _, err := s.repo.Save(sh); err != nil {
		return nil, err
	}

	return newFiles, nil
}

func (s *ShareService) CreateUploadLink(sh *Share, req UploadLinkRequest) (*UploadLink, error) {
	if sh.Trashed() {
		return nil, ErrShareTrashed
	}

	ul, err := NewUploadLink(req)
	if err != nil {
		return nil, err
	}

	sh.UploadLinks = append(sh.UploadLinks, *ul)

	if _, err := s.repo.Save(sh); err != nil {
		return nil, err
	}

	return ul, nil
}

func (s *ShareService) RevokeUploadLink(sh *Share, id string) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	idx := -1
	for i := range sh.UploadLinks {
		if sh.UploadLinks[i].ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrUploadLinkNotFound
	}

	sh.UploadLinks = append(sh.UploadLinks[:idx], sh.UploadLinks[idx+1:]...)

	_, err := s.repo.Save(sh)

	return err
}

func (s *ShareService) GetShareByHandle(handle string) (*Share, error) {
	return s.repo.GetByHandle(handle)
}

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

func (s *ShareService) GetFileUploadState(sh *Share, fileID string) (FileUploadState, error) {
	return s.store.FileUploadState(sh.ID, fileID)
}

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

func (s *ShareService) DownloadAll(sh *Share, w io.Writer) error {
	return s.store.CreateShareZipStream(*sh, w)
}

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

	_, err := s.repo.Save(sh)

	return err
}

func (s *ShareService) TrashShare(sh *Share) error {
	if sh.Trashed() {
		return ErrShareTrashed
	}

	now := time.Now().UTC()
	sh.TrashedAt = &now

	_, err := s.repo.Save(sh)

	return err
}

func (s *ShareService) RecoverShare(sh *Share) error {
	if !sh.Trashed() {
		return ErrShareNotTrashed
	}

	sh.TrashedAt = nil

	_, err := s.repo.Save(sh)

	return err
}

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
