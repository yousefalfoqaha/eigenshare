package share

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"

	"eigenshare/backend/api"

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
	success := false

	if err := os.MkdirAll(tmpDirPath, 0o755); err != nil {
		return nil, api.NewError(http.StatusInternalServerError, "failed to create temporary directory", err)
	}

	defer func() {
		os.RemoveAll(tmpDirPath)
		if !success {
			os.RemoveAll("data/" + share.ID)
		}
	}()

	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, api.NewError(http.StatusBadRequest, "failed to read multipart data", err)
		}

		switch part.FormName() {
		case "metadata":
			if err := json.NewDecoder(part).Decode(&share); err != nil {
				return nil, api.NewError(http.StatusBadRequest, "invalid metadata", err)
			}

			if err := validateShareID(share.ID); err != nil {
				return nil, err
			}
		case "file":
			fileID := uuid.New()
			fileDir, err := os.Create(tmpDirPath + "/" + fileID.String())
			if err != nil {
				return nil, api.NewError(http.StatusInternalServerError, "failed to create file", err)
			}

			size, err := io.Copy(fileDir, part)
			if err != nil {
				return nil, api.NewError(http.StatusInternalServerError, "failed to write file", err)
			}

			files = append(files, File{ID: fileID.String(), Name: part.FileName(), Size: int(size)})
		}

		part.Close()
	}

	share.Files = files
	shareDirPath := "data/" + share.ID

	if err := os.Rename(tmpDirPath, shareDirPath); err != nil {
		return nil, api.NewError(http.StatusInternalServerError, "failed to finalize share directory", err)
	}

	if err := s.shareRepository.Save(share); err != nil {
		return nil, api.NewError(http.StatusInternalServerError, "failed to save share", err)
	}

	success = true
	return &share, nil
}

func validateShareID(id string) error {
	if id == "" {
		return api.NewError(http.StatusBadRequest, "share ID is required", nil)
	}
	if !validShareID.MatchString(id) {
		return api.NewError(http.StatusBadRequest, "share ID may only contain letters, numbers, hyphens and underscores (max 64 chars)", nil)
	}
	return nil
}
