package service

import (
	"database/sql"
	"eigenshare/backend/api"
	"eigenshare/backend/repository"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
)

type ShareService struct {
	shareRepository *repository.ShareRepository
	fileStorage     *storage.FileStorage
}

func NewShareService(repo *repository.ShareRepository, fs *storage.FileStorage) *ShareService {
	return &ShareService{
		shareRepository: repo,
		fileStorage:     fs,
	}
}

func (s *ShareService) StreamFile(shareID string, fileID string, offset int64, stream io.Reader) error {
	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			return api.NewError(http.StatusNotFound, "No share found")
		}

		return err
	}

	var file *share.File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return api.NewError(http.StatusNotFound, "No file found")
	}

	if offset >= file.ExpectedSize {
		return api.NewError(http.StatusConflict, "Chunk offset exceeds declared file size")
	}

	err = s.fileStorage.WriteFileStream(shareID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidChunkOffset) {
			return api.NewError(http.StatusBadRequest, "Invalid chunk offset")
		}
		if errors.Is(err, storage.ErrFileNotFound) {
			return api.NewError(http.StatusBadRequest, "File not found")
		}

		return err
	}

	return nil
}

type CreateShareParams struct {
	Handle string
	Files  []CreateFileParams
}

type CreateFileParams struct {
	Name         string
	ExpectedSize int64
	Order        int
}

func (s *ShareService) RequestShare(params CreateShareParams) (*share.Share, error) {
	sh, err := share.NewShare(uuid.New().String(), params.Handle)
	if err != nil {
		return nil, api.NewError(http.StatusBadRequest, err.Error())
	}

	var success bool
	defer func() {
		if !success {
			s.fileStorage.RemoveShareFolder(sh.ID)
		}
	}()

	for _, f := range params.Files {
		file, err := share.NewFile(f.Name, f.ExpectedSize, f.Order)
		if err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}

		file.ID = uuid.New().String()
		if err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}

		sh.Files = append(sh.Files, *file)
	}

	if err := s.fileStorage.CreateShareFolder(*sh); err != nil {
		return nil, err
	}

	err = s.shareRepository.MakeTransaction(func(tx *sql.Tx) error {
		if _, err := s.shareRepository.Save(*sh, tx); err != nil {
			if errors.Is(err, share.ErrHandleExists) {
				return api.NewError(http.StatusConflict, err.Error())
			}

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

func (s *ShareService) GetShares() ([]share.ShareSummary, error) {
	return s.shareRepository.GetAll()
}

func (s *ShareService) GetShare(id string) (*share.Share, error) {
	if id == "" {
		return nil, api.NewError(http.StatusBadRequest, "share ID is required")
	}

	sh, err := s.shareRepository.Get(id)
	if err == sql.ErrNoRows {
		return nil, api.NewError(http.StatusNotFound, "No share found")
	}
	if err != nil {
		return nil, err
	}

	return sh, nil
}

func (s *ShareService) GetShareProgress(id string) (int64, error) {
	sh, err := s.shareRepository.Get(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, api.NewError(http.StatusNotFound, "No share found")
		}
		return 0, err
	}

	bytesWritten, err := s.fileStorage.GetShareBytesWritten(*sh)
	if errors.Is(err, storage.ErrFileNotFound) {
		return 0, api.NewError(http.StatusBadRequest, "No share folder found")
	}

	return bytesWritten, nil
}

func (s *ShareService) DownloadAll(shareID string) (string, error) {
	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", api.NewError(http.StatusNotFound, "No share found")
		}
		return "", err
	}

	isComplete, err := s.fileStorage.IsShareComplete(*sh)
	if errors.Is(err, storage.ErrFileNotFound) {
		return "", api.NewError(http.StatusBadRequest, "No share folder found")
	}

	if !isComplete {
		return "", api.NewError(http.StatusConflict, "Share is not completed")
	}

	return s.fileStorage.SharePath(sh.ID), nil
}
