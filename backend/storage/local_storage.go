package storage

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type LocalStorage struct {
	tmpDir    string
	chunkSize int64
}

func NewLocalStorage(tmpDir string, chunkSize int64) *LocalStorage {
	return &LocalStorage{
		tmpDir:    tmpDir,
		chunkSize: chunkSize,
	}
}

func (s *LocalStorage) WriteChunk(shareID string, fileIndex int, chunkIndex int, chunkReader io.Reader) error {
	filePath := filepath.Join(s.tmpDir, shareID, strconv.Itoa(fileIndex))
	f, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	offset := int64(chunkIndex) * s.chunkSize
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	if _, err := io.Copy(f, io.LimitReader(chunkReader, s.chunkSize)); err != nil {
		return err
	}

	return nil
}
