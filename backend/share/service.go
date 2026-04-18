package share

import (
	"database/sql"
	"eigenshare/backend/api"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/uuid"
)

type Service struct {
	ShareRepository *Repository
}

var validShareHandle = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func (s *Service) uploadChunk(shareId string, fileIndex int, chunkIndex int, chunk io.Reader) error {
	if shareId == "" {
		return api.NewError(http.StatusBadRequest, "Invalid share ID")
	}

	share, err := s.ShareRepository.Get(shareId)
	if err != nil {
		return api.NewError(http.StatusNotFound, "No share found")
	}

	file := new(File)

	for _, f := range share.Files {
		if f.Index == fileIndex {
			*file = f
			break
		}
	}

	if file == nil {
		return api.NewError(http.StatusNotFound, "No file found")
	}
}

func (s *Service) requestShare(req shareRequest) (*Share, error) {
	var share Share

	share.ID = uuid.New().String()
	share.Handle = req.Handle
	share.Status = "PENDING"

	var files []File
	var totalSize int64 = 0

	tmpPath := filepath.Join("data/tmp/", share.ID)

	err := os.Mkdir(tmpPath, 0o77)
	if err != nil {
		return nil, err
	}

	for _, f := range req.Files {
		totalSize += f.Size

		file := File{Index: f.Index, Name: f.Name, Size: f.Size}

		err := file.createBitmask()
		if err != nil {
			return nil, err
		}

		osFile, err := os.Create(filepath.Join(tmpPath, string(f.Index)))
		if err != nil {
			return nil, err
		}

		if err := osFile.Truncate(f.Size); err != nil {
			osFile.Close()
			return nil, err
		}
		osFile.Close()

		files = append(files, file)
	}

	share.Files = files

	if _, err := s.ShareRepository.Save(share); err != nil {
		os.RemoveAll(tmpPath)
		return nil, err
	}

	return &share, nil
}

func (s *Service) getShares() ([]ShareSummary, error) {
	shares, err := s.ShareRepository.GetAll()
	if err != nil {
		return nil, err
	}

	return shares, nil
}

func (s *Service) getShare(id string) (*Share, error) {
	if id == "" {
		return nil, api.NewError(http.StatusBadRequest, "share ID is required")
	}

	share, err := s.ShareRepository.Get(id)
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
