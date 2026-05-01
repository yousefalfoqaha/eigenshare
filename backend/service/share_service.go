package service

import (
	"database/sql"
	"eigenshare/backend/api"
	"eigenshare/backend/repository"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
)

type ShareService struct {
	shareRepository *repository.ShareRepository
	fileStorage     *storage.FileStorage
	uploadTracker   *upload.UploadTracker
	shareWorker     *UploadWorker
}

func NewShareService(repo *repository.ShareRepository, fs *storage.FileStorage, ut *upload.UploadTracker, sw *UploadWorker) *ShareService {
	return &ShareService{
		shareRepository: repo,
		fileStorage:     fs,
		uploadTracker:   ut,
		shareWorker:     sw,
	}
}

func (s *ShareService) UploadChunk(shareID string, fileIndex int, chunkIndex int, chunk io.Reader) error {
	if shareID == "" {
		return api.NewError(http.StatusBadRequest, "Share ID is empty")
	}

	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			return api.NewError(http.StatusNotFound, "No share found")
		}

		return err
	}

	if sh.Status != share.STATUS_UPLOADING {
		return api.NewError(http.StatusConflict, "Share is not uploading")
	}

	var file *share.File
	for i, f := range sh.Files {
		if f.Index == fileIndex {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return api.NewError(http.StatusNotFound, "No file found")
	}

	err = s.fileStorage.WriteChunk(shareID, fileIndex, chunkIndex, chunk)
	if err != nil {
		return err
	}

	s.uploadTracker.MarkChunk(shareID, fileIndex, chunkIndex)

	return nil
}

type CreateShareParams struct {
	Handle string
	Files  []CreateFileParams
}

type CreateFileParams struct {
	Name  string
	Size  int64
	Index int
}

func (s *ShareService) RequestShare(params CreateShareParams) (*share.Share, error) {
	sh, err := share.NewShare(uuid.New().String(), params.Handle, share.STATUS_UPLOADING)
	if err != nil {
		return nil, api.NewError(http.StatusBadRequest, err.Error())
	}

	var success bool
	defer func() {
		if !success {
			s.fileStorage.RemoveShareFolder(sh.ID)
			s.uploadTracker.UntrackShareUpload(sh.ID)
		}
	}()

	if err := s.fileStorage.CreateShareFolder(sh.ID); err != nil {
		return nil, err
	}

	for _, f := range params.Files {
		file, err := share.NewFile(f.Name, f.Size, f.Index)
		if err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}

		sh.Files = append(sh.Files, *file)

		if err := s.fileStorage.CreateFile(sh.ID, f.Index, f.Size); err != nil {
			return nil, err
		}

	}

	if err := s.uploadTracker.TrackShareUpload(*sh); err != nil {
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

func (s *ShareService) GetUploadStatus(id string) (*upload.ShareUploadStatus, error) {
	status := s.uploadTracker.GetShareUploadStatus(id)
	return &status, nil
}

func (s *ShareService) ResumeShare(shareID string) (*share.Share, error) {
	if shareID == "" {
		return nil, api.NewError(http.StatusBadRequest, "Share ID is required")
	}

	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, api.NewError(http.StatusNotFound, "No share found")
		}
		return nil, err
	}

	if sh.Status != share.STATUS_PENDING {
		return nil, api.NewError(http.StatusConflict, "Share is not in pending state")
	}

	if err := s.shareWorker.RequestStatusUpdate(shareID, share.STATUS_UPLOADING); err != nil {
		return nil, err
	}

	sh.Status = share.STATUS_UPLOADING
	return sh, nil
}

func (s *ShareService) Download(shareID string, fileIndex int) (string, error) {
	if shareID == "" {
		return "", api.NewError(http.StatusBadRequest, "Share ID is required")
	}

	sh, err := s.shareRepository.Get(shareID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", api.NewError(http.StatusNotFound, "No share found")
		}
		return "", err
	}

	if sh.Status == share.STATUS_COMPLETE {
		return s.fileStorage.ShareFilePath(sh.ID, fileIndex), nil
	}

	if sh.Status == share.STATUS_UPLOADING && s.uploadTracker.IsShareComplete(shareID) {
		if err := s.shareWorker.RequestStatusUpdate(shareID, share.STATUS_COMPLETE); err != nil {
			return "", err
		}

		return s.fileStorage.ShareFilePath(sh.ID, fileIndex), nil
	}

	return "", api.NewError(http.StatusConflict, "Share is not yet complete")
}
