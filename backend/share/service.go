package share

import (
	"database/sql"
	"eigenshare/backend/api"
	"eigenshare/backend/storage"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
)

type Service struct {
	shareRepository *Repository
	localStorage    *storage.LocalStorage
	uploadManager   *UploadManager
}

func NewService(repo *Repository, ls *storage.LocalStorage, um *UploadManager) *Service {
	return &Service{
		shareRepository: repo,
		localStorage:    ls,
		uploadManager:   um,
	}
}

func (s *Service) uploadChunk(shareID string, fileIndex int, chunkIndex int, chunk io.Reader) error {
	if shareID == "" {
		return api.NewError(http.StatusBadRequest, "Share ID is empty")
	}

	share, err := s.shareRepository.Get(shareID)
	if err == sql.ErrNoRows {
		return api.NewError(http.StatusNotFound, "No share found")
	}
	if err != nil {
		return err
	}

	if share.Status == STATUS_COMPLETE {
		return api.NewError(http.StatusConflict, "Share is already complete")
	}

	var file *File
	for i, f := range share.Files {
		if f.Index == fileIndex {
			file = &share.Files[i]
			break
		}
	}
	if file == nil {
		return api.NewError(http.StatusNotFound, "No file found")
	}

	err = s.localStorage.WriteChunk(shareID, fileIndex, chunkIndex, chunk)
	if err != nil {
		return err
	}

	chunksUploaded, totalChunks, err := s.uploadManager.UploadChunk(shareID, fileIndex, chunkIndex)
	if err != nil {
		return err
	}

	if chunksUploaded < totalChunks {
		return nil
	}

	err = s.shareRepository.makeTransaction(func(tx *sql.Tx) error {
		share.Status = STATUS_COMPLETE

		if _, err := s.shareRepository.Save(*share, tx); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) requestShare(req shareRequest) (*Share, error) {
	share, err := NewShare(uuid.New().String(), req.Handle)
	if err != nil {
		return nil, api.NewError(http.StatusBadRequest, err.Error())
	}

	var success bool
	defer func() {
		if !success {
			s.localStorage.RemoveShareFolder(share.ID)
			s.uploadManager.UntrackShareUpload(share.ID)
		}
	}()

	var files []File
	var totalSize int64

	for _, f := range req.Files {
		file, err := NewFile(f.Name, f.Size, f.Index)
		if err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}

		totalSize += file.Size
		files = append(files, *file)
	}

	share.Files = files

	// TODO: implement max size check
	_ = totalSize

	if err := s.localStorage.CreateShareFolder(share.ID); err != nil {
		return nil, err
	}

	for _, f := range files {
		if err := s.localStorage.CreateFile(share.ID, f.Index, f.Size); err != nil {
			return nil, err
		}
	}

	if err := s.uploadManager.TrackShareUpload(*share); err != nil {
		return nil, err
	}

	err = s.shareRepository.makeTransaction(func(tx *sql.Tx) error {
		if _, err := s.shareRepository.Save(*share, tx); err != nil {
			if errors.Is(err, ErrHandleExists) {
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
	return share, nil
}

func (s *Service) getShares() ([]ShareSummary, error) {
	return s.shareRepository.GetAll()
}

func (s *Service) getShare(id string) (*Share, error) {
	if id == "" {
		return nil, api.NewError(http.StatusBadRequest, "share ID is required")
	}

	share, err := s.shareRepository.Get(id)
	if err == sql.ErrNoRows {
		return nil, api.NewError(http.StatusNotFound, "No share found")
	}
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (s *Service) getUploadStatus(id string) (*ShareUploadStatus, error) {
	status := s.uploadManager.GetShareUploadStatus(id)
	return &status, nil
}
