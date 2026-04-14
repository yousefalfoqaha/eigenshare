package share

import (
	"database/sql"
	"eigenshare/backend/api"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"

	"github.com/google/uuid"
)

type Service struct {
	shareRepository *Repository
}

func NewService(shareRepository *Repository) *Service {
	return &Service{shareRepository}
}

var validShareID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func (s *Service) share(r *multipart.Reader) (*Share, error) {
	var share Share
	var files []File
	tmpDirPath := "data/tmp/" + uuid.New().String()

	if err := os.MkdirAll(tmpDirPath, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDirPath)

	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		switch part.FormName() {
		case "metadata":
			if err := json.NewDecoder(part).Decode(&share); err != nil {
				return nil, err
			}
			if share.ID == "" {
				return nil, api.NewError(http.StatusBadRequest, "Share ID is empty")
			}
			if err := validateShareID(share.ID); err != nil {
				return nil, err
			}
		case "file":
			fileID := uuid.New()
			fileDir, err := os.Create(tmpDirPath + "/" + fileID.String())
			if err != nil {
				return nil, err
			}

			size, err := io.Copy(fileDir, part)
			if err != nil {
				return nil, err
			}

			files = append(files, File{ID: fileID.String(), Name: part.FileName(), Size: int(size)})
		}

		part.Close()
	}

	share.Files = files
	shareDirPath := "data/" + share.ID

	exists, err := s.shareRepository.Exists(share.ID)
	if err != nil {
		return nil, err
	}

	if exists {
		return nil, api.NewError(http.StatusConflict, "Share with ID "+share.ID+" exists")
	}

	if err := os.Rename(tmpDirPath, shareDirPath); err != nil {
		return nil, err
	}

	if err := s.shareRepository.Save(share); err != nil {
		os.RemoveAll(shareDirPath)
		return nil, err
	}

	return &share, nil
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

func validateShareID(id string) error {
	if id == "" {
		return api.NewError(http.StatusBadRequest, "share ID is required")
	}
	if !validShareID.MatchString(id) {
		return api.NewError(http.StatusBadRequest, "share ID may only contain letters, numbers, hyphens and underscores (max 64 chars)")
	}
	return nil
}
