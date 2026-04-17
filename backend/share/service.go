package share

import (
	"database/sql"
	"eigenshare/backend/api"
	"fmt"
	"net/http"
	"regexp"

	"github.com/google/uuid"
)

type Service struct {
	shareRepository *Repository
}

func NewService(shareRepository *Repository) *Service {
	return &Service{shareRepository}
}

var validShareHandle = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func (s *Service) requestShare(req shareRequest) (*Share, error) {
	var share Share

	share.ID = uuid.New().String()
	share.Handle = req.Handle
	share.Status = "PENDING"

	var files []File
	for _, f := range req.Files {
		fileID := uuid.New().String()
		bitmask, numChunks, err := createBitmask(f.Size)
		if err != nil {
			return nil, err
		}

		file := File{ID: fileID, Name: f.Name, Chunks: numChunks, Bitmask: bitmask, Size: f.Size}
		files = append(files, file)
	}

	share.Files = files

	if _, err := s.shareRepository.Save(share); err != nil {
		return nil, err
	}

	return &share, nil
}

func createBitmask(fileSize int64) ([]byte, int, error) {
	if fileSize <= 0 {
		return nil, 0, fmt.Errorf("Invalid file size")
	}

	const CHUNK_SIZE_MB = 5
	var chunkSize int64 = 1024 * 1024 * CHUNK_SIZE_MB

	numChunks := (fileSize + chunkSize - 1) / chunkSize
	numBytes := (numChunks + 7) / 8

	return make([]byte, numBytes), int(numChunks), nil
}

func (s *Service) getShares() ([]ShareSummary, error) {
	shares, err := s.shareRepository.GetAll()
	if err != nil {
		return nil, err
	}

	return shares, nil
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

func validateShareID(handle string) error {
	if handle == "" {
		return api.NewError(http.StatusBadRequest, "share handle is required")
	}
	if !validShareHandle.MatchString(handle) {
		return api.NewError(http.StatusBadRequest, "share handle may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}
	return nil
}
