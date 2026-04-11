package share

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"os"

	"github.com/google/uuid"
)

type Service struct {
	shareRepository *Repository
}

func NewService(shareRepository *Repository) *Service {
	return &Service{shareRepository}
}

func (s *Service) share(r *multipart.Reader) (*Share, error) {
	var share Share
	var files []File
	tmpDirUUID := uuid.New()

	tmpDir, err := os.Create("/data/tmp/" + tmpDirUUID.String())
	if err != nil {
		return nil, err
	}

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
			json.NewDecoder(part).Decode(&share)
			_, err := os.Create("data/" + share.ID)
			if err != nil {
				return nil, err
			}
		case "file":
			fileID := uuid.New()
			fileDir, err := os.Create(tmpDir.Name() + "/" + fileID.String())
			if err != nil {
				return nil, err
			}

			// TODO: check for unique naming -> add filname (n), n being number of same-name files (regex of %filename% (*))

			size, err := io.Copy(fileDir, part)
			if err != nil {
				return nil, err
			}

			file := File{ID: fileID.String(), Name: part.FileName(), Size: int(size)}

			files = append(files, file)
		}

		part.Close()
	}

	share = Share{
		Files: files,
	}

	shareDir := "/data/" + share.ID

	if err := os.Rename(tmpDir.Name(), shareDir); err != nil {
		return nil, err
	}

	if err := s.shareRepository.Save(share); err != nil {
		os.RemoveAll(shareDir)
		return nil, err
	}

	return &share, nil
}
