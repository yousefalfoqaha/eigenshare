package share

import (
	"database/sql"
	"errors"
	"fmt"
	"io"

	"eigenshare/internal/timestamp"

	"github.com/google/uuid"
)

type ShareService struct {
	repo    *ShareRepository
	storage *FileStorage
}

func NewShareService(repo *ShareRepository, fs *FileStorage) *ShareService {
	return &ShareService{
		repo:    repo,
		storage: fs,
	}
}

func (s *ShareService) StreamFile(shareID string, fileID string, offset int64, stream io.Reader) (int64, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrShareNotFound
		}
		return 0, err
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

	bytesWritten, err := s.storage.WriteFileStream(shareID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		return 0, err
	}

	return bytesWritten, nil
}

func (s *ShareService) RequestShare(req ShareRequest) (*Share, error) {
	sh, err := NewShare("", req.Handle, req.ExpiryMinutes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	return s.repo.Save(sh)
}

func (s *ShareService) UploadFiles(shareID string, fileReqs []FileRequest) ([]File, error) {
	sh, err := s.GetShare(shareID)
	if err != nil {
		return nil, err
	}

	var newFiles []File

	for _, req := range fileReqs {
		file, err := NewFile(req.Name, req.ExpectedSize)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
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

func (s *ShareService) GetShareByHandle(handle string) (*Share, error) {
	sh, err := s.repo.GetByHandle(handle)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}
	return sh, nil
}

func (s *ShareService) Delete(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	if err := s.repo.Delete(ids...); err != nil {
		return err
	}

	return nil
}

func (s *ShareService) GetShares() ([]ShareSummary, error) {
	return s.repo.GetAll()
}

func (s *ShareService) GetTrashedShares() ([]ShareSummary, error) {
	return s.repo.GetAllTrashed()
}

func (s *ShareService) GetShare(id string) (*Share, error) {
	sh, err := s.repo.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}
	return sh, nil
}

func (s *ShareService) GetShareProgress(id string) (map[string]int64, error) {
	exists, err := s.repo.Exists(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrShareNotFound
	}

	return s.storage.GetShareBytesWritten(id)
}

func (s *ShareService) DownloadAll(shareID string, w io.Writer) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
	}

	return s.storage.CreateShareZipStream(*sh, w)
}

func (s *ShareService) DownloadFile(shareID string, fileID string) (string, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrShareNotFound
		}
		return "", err
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return "", ErrFileNotFound
	}

	isComplete, err := s.storage.IsShareComplete(*sh)
	if err != nil {
		return "", err
	}

	if !isComplete {
		return "", ErrShareNotComplete
	}

	return s.storage.ShareFilePath(sh.ID, file.ID)
}

func (s *ShareService) DeleteFile(shareID, fileID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
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

	if _, err := s.repo.Save(sh); err != nil {
		return err
	}

	return nil
}

func (s *ShareService) TrashShare(shareID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
	}

	now := timestamp.Now()
	sh.TrashedAt = &now

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) RecoverShare(shareID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
	}

	sh.TrashedAt = nil

	_, err = s.repo.Save(sh)

	return err
}
