package share

import (
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
)

type ShareService struct {
	shareRepository *ShareRepository
	fileStorage     *FileStorage
}

func NewShareService(repo *ShareRepository, fs *FileStorage) *ShareService {
	return &ShareService{
		shareRepository: repo,
		fileStorage:     fs,
	}
}

func (s *ShareService) StreamFile(shareID string, fileID string, offset int64, stream io.Reader) error {
	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return ErrFileNotFound
	}

	if offset >= file.ExpectedSize {
		return ErrChunkOffsetExceedsSize
	}

	err = s.fileStorage.WriteFileStream(shareID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		return err
	}

	return nil
}

func (s *ShareService) RequestShare(req ShareRequest) (*Share, error) {
	sh, err := NewShare(uuid.New().String(), req.Handle, req.ExpiryMinutes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	var success bool
	defer func() {
		if !success {
			s.fileStorage.RemoveShareFolder(sh.ID)
		}
	}()

	for _, f := range req.Files {
		file, err := NewFile(f.Name, f.ExpectedSize, f.Order)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}

		file.ID = uuid.New().String()
		sh.Files = append(sh.Files, *file)
	}

	if err := s.fileStorage.CreateShareFolder(*sh); err != nil {
		return nil, err
	}

	err = s.shareRepository.MakeTransaction(func(tx *sql.Tx) error {
		if _, err := s.shareRepository.Save(*sh, tx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	success = true
	return sh, nil
}

func (s *ShareService) GetShareByHandle(handle string) (*Share, error) {
	sh, err := s.shareRepository.GetByHandle(handle)
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

	err := s.shareRepository.MakeTransaction(func(tx *sql.Tx) error {
		return s.shareRepository.Delete(ids, tx)
	})
	if err != nil {
		return err
	}

	for _, id := range ids {
		s.fileStorage.RemoveShareFolder(id)
	}
	return nil
}

func (s *ShareService) GetShares() ([]ShareSummary, error) {
	return s.shareRepository.GetAll()
}

func (s *ShareService) GetShare(id string) (*Share, error) {
	sh, err := s.shareRepository.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrShareNotFound
		}
		return nil, err
	}
	return sh, nil
}

func (s *ShareService) GetShareProgress(id string) (int64, error) {
	sh, err := s.shareRepository.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrShareNotFound
		}
		return 0, err
	}

	bytesWritten, err := s.fileStorage.GetShareBytesWritten(*sh)
	if err != nil {
		return 0, err
	}

	return bytesWritten, nil
}

func (s *ShareService) DownloadAll(shareID string, w io.Writer) error {
	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
	}

	return s.fileStorage.CreateShareZipStream(*sh, w)
}

func (s *ShareService) DownloadFile(shareID string, fileID string) (string, error) {
	sh, err := s.shareRepository.Get(shareID)
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

	isComplete, err := s.fileStorage.IsShareComplete(*sh)
	if err != nil {
		return "", err
	}

	if !isComplete {
		return "", ErrShareNotComplete
	}

	return s.fileStorage.ShareFilePath(sh.ID, file.ID)
}

func (s *ShareService) DeleteFile(handle, fileID string) error {
	sh, err := s.shareRepository.GetByHandle(handle)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrShareNotFound
		}
		return err
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return ErrFileNotFound
	}

	if err := s.shareRepository.DeleteFile(sh.ID, fileID); err != nil {
		return err
	}

	return s.fileStorage.RemoveFile(sh.ID, fileID)
}

func (s *ShareService) GetExpiredShares() ([]string, error) {
	return s.shareRepository.GetExpiredIDs()
}
