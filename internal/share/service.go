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

func (s *ShareService) StreamFile(shareID string, fileID string, offset int64, stream io.Reader) (FileProgress, error) {
	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FileProgress{}, ErrShareNotFound
		}
		return FileProgress{}, err
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return FileProgress{}, ErrFileNotFound
	}

	if offset >= file.ExpectedSize {
		return FileProgress{}, ErrChunkOffsetExceedsSize
	}

	bytesWritten, err := s.fileStorage.WriteFileStream(shareID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		return FileProgress{}, err
	}

	return FileProgress{
		FileID:       file.ID,
		BytesWritten: bytesWritten,
		ExpectedSize: file.ExpectedSize,
		Complete:     bytesWritten == file.ExpectedSize,
	}, nil
}

func (s *ShareService) RequestShare(req ShareRequest) (*Share, error) {
	sh, err := NewShare("", req.Handle, req.ExpiryMinutes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	saved := false
	var success bool
	defer func() {
		if !success {
			if saved {
				_ = s.shareRepository.Delete(sh.ID)
			}
			_ = s.fileStorage.RemoveShareFolder(sh.ID)
		}
	}()

	for _, f := range req.Files {
		file, err := NewFile(f.Name, f.ExpectedSize)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		}

		file.ID = uuid.New().String()
		sh.Files = append(sh.Files, *file)
	}

	sh, err = s.shareRepository.Save(sh)
	if err != nil {
		return nil, err
	}
	saved = true

	if err := s.fileStorage.CreateShareFolder(*sh); err != nil {
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

	for _, id := range ids {
		if err := s.shareRepository.Delete(id); err != nil {
			return err
		}
		if err := s.fileStorage.RemoveShareFolder(id); err != nil {
			return err
		}
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

func (s *ShareService) AddFiles(handle string, fileReqs []FileRequest) ([]File, error) {
	sh, err := s.GetShareByHandle(handle)
	if err != nil {
		return nil, err
	}

	incomplete, err := s.fileStorage.HasIncompleteFiles(*sh)
	if err != nil {
		return nil, err
	}
	if incomplete {
		return nil, ErrShareUploading
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

	if _, err := s.shareRepository.Save(sh); err != nil {
		return nil, err
	}

	return newFiles, nil
}

func (s *ShareService) DeleteFile(handle, fileID string) error {
	sh, err := s.shareRepository.GetByHandle(handle)
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

	if _, err := s.shareRepository.Save(sh); err != nil {
		return err
	}

	if err := s.fileStorage.RemoveFile(sh.ID, removedFileID); errors.Is(err, ErrFileNotFound) {
		return nil
	} else {
		return err
	}
}

func (s *ShareService) GetExpiredShares() ([]string, error) {
	return s.shareRepository.GetExpiredIDs()
}
