package share

import (
	"fmt"
	"io"
	"slices"

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

func (s *ShareService) StreamFile(shareID string, fileID string, offset int64, stream io.Reader, u *user.User) (int64, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return 0, err
	}
	if !sh.CanManage(u) {
		return 0, ErrShareNotFound
	}
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

	bytesWritten, err := s.store.WriteFileStream(shareID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		return 0, err
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

	return s.repo.Save(sh)
}

func (s *ShareService) UploadFiles(shareID string, fileReqs []FileRequest, u *user.User) ([]File, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return nil, err
	}
	if !sh.CanManage(u) {
		return nil, ErrShareNotFound
	}
	if sh.Trashed() {
		return nil, ErrShareTrashed
	}

	var newFiles []File

	for _, req := range fileReqs {
		file, err := NewFile(req.Name, req.ExpectedSize)
		if err != nil {
			return nil, err
		}
		file.ID = uuid.New().String()
		sh.Files = append(sh.Files, *file)
		newFiles = append(newFiles, *file)
	}

	if _, err := s.repo.Save(sh); err != nil {
		return nil, err
	}

	return newFiles, nil
}

func (s *ShareService) GetShareByHandle(handle string, u *user.User) (*Share, error) {
	sh, err := s.repo.GetByHandle(handle)
	if err != nil {
		return nil, err
	}
	if !sh.CanView(u) {
		return nil, ErrShareNotFound
	}

	return sh, nil
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

func (s *ShareService) GetShare(id string, u *user.User) (*Share, error) {
	sh, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if !sh.CanManage(u) {
		return nil, ErrShareNotFound
	}
	return sh, nil
}

func (s *ShareService) GetUploadState(sh *Share, u *user.User) (*UploadState, error) {
	if !sh.CanView(u) {
		return nil, ErrShareNotFound
	}

	return s.store.UploadState(sh.ID)
}

func (s *ShareService) DownloadAll(shareID string, w io.Writer) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}

	return s.store.CreateShareZipStream(*sh, w)
}

func (s *ShareService) DownloadFile(shareID string, fileID string) (*File, string, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return nil, "", err
	}

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

	upload, err := s.store.UploadState(sh.ID)
	if err != nil {
		return nil, "", err
	}

	if !upload.Uploaded(*file) {
		return nil, "", ErrFileNotComplete
	}

	path, err := s.store.ShareFilePath(sh.ID, file.ID)
	if err != nil {
		return nil, "", err
	}

	return file, path, nil
}

func (s *ShareService) DeleteFile(shareID, fileID string, u *user.User) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	var removedFileID string
	for i, f := range sh.Files {
		if f.ID == fileID {
			removedFileID = f.ID
			sh.Files = append(sh.Files[:i], sh.Files[i+1:]...)
			break
		}
	}
	if removedFileID == "" {
		return ErrFileNotFound
	}

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) TrashShare(shareID string, u *user.User) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	now := timestamp.Now()
	sh.TrashedAt = &now

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) RecoverShare(shareID string, u *user.User) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if !sh.Trashed() {
		return ErrShareNotTrashed
	}

	sh.TrashedAt = nil

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) AddOwner(shareID string, u *user.User, newOwnerID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
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

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) RemoveOwner(shareID string, u *user.User, ownerID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
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

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) SetVisibility(shareID string, u *user.User, visibility Visibility) error {
	if !visibility.Valid() {
		return fmt.Errorf("visibility must be public or private: %w", ErrInvalidVisibility)
	}

	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	if sh.Visibility == visibility {
		return nil
	}

	sh.Visibility = visibility

	_, err = s.repo.Save(sh)

	return err
}
