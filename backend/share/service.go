package share

import (
	"database/sql"
	"eigenshare/backend/api"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
)

type Service struct {
	ShareRepository *Repository
}

const (
	TMP_PATH   = "data/tmp"
	CHUNK_SIZE = 5 * 1024 * 1024
)

func (s *Service) uploadChunk(shareId string, fileIndex int, chunkIndex int, chunk io.Reader) error {
	if shareId == "" {
		return api.NewError(http.StatusBadRequest, "Share ID is empty")
	}

	share, err := s.ShareRepository.Get(shareId)
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

	filePath := filepath.Join(TMP_PATH, share.ID, strconv.Itoa(file.Index))
	f, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	offset := int64(chunkIndex) * CHUNK_SIZE
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	if _, err := io.Copy(f, io.LimitReader(chunk, CHUNK_SIZE)); err != nil {
		return err
	}

	err = s.ShareRepository.makeTransaction(func(tx *sql.Tx) error {
		if err := file.markChunkUploaded(chunkIndex); err != nil {
			return err
		}

		share.Status = STATUS_UPLOADING
		if _, err := s.ShareRepository.Save(*share, tx); err != nil {
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
	var share Share

	share.ID = uuid.New().String()
	share.Status = STATUS_PENDING

	err := share.setHandle(req.Handle)
	if err != nil {
		return nil, api.NewError(http.StatusBadRequest, err.Error())
	}

	sharePath := filepath.Join(TMP_PATH, share.ID)
	if err := os.MkdirAll(sharePath, 0755); err != nil {
		return nil, err
	}

	var success bool
	defer func() {
		if !success {
			os.RemoveAll(sharePath)
		}
	}()

	var files []File
	var totalSize int64

	for _, f := range req.Files {
		totalSize += f.Size

		var file File
		if err := file.setName(f.Name); err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}
		if err := file.setSize(f.Size); err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}
		if err := file.setIndex(f.Index); err != nil {
			return nil, api.NewError(http.StatusBadRequest, err.Error())
		}
		if err := file.createBitmask(); err != nil {
			return nil, err
		}

		osFile, err := os.Create(filepath.Join(sharePath, strconv.Itoa(f.Index)))
		if err != nil {
			return nil, err
		}

		truncErr := osFile.Truncate(f.Size)
		osFile.Close()
		if truncErr != nil {
			return nil, truncErr
		}

		files = append(files, file)
	}

	share.Files = files

	// TODO: implement max size check
	_ = totalSize

	err = s.ShareRepository.makeTransaction(func(tx *sql.Tx) error {
		if _, err := s.ShareRepository.Save(share, tx); err != nil {
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
	return &share, nil
}

func (s *Service) getShares() ([]ShareSummary, error) {
	return s.ShareRepository.GetAll()
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
