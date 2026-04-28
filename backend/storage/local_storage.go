package storage

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type FileStorage struct {
	tmpDir    string
	dataDir   string
	chunkSize int64
}

func NewFileStorage(dataDir string, tmpDir string, chunkSize int64) *FileStorage {
	return &FileStorage{dataDir, tmpDir, chunkSize}
}

func (s *FileStorage) PersistShareFolder(shareID string) error {
	err := os.Rename(filepath.Join(s.tmpDir, shareID), filepath.Join(s.dataDir, shareID))
	if err != nil {
		return err
	}

	return nil
}

func (s *FileStorage) CreateShareFolder(shareID string) error {
	return os.MkdirAll(filepath.Join(s.tmpDir, shareID), 0755)
}

func (s *FileStorage) CreateFile(shareID string, fileIndex int, size int64) error {
	filePath := filepath.Join(s.tmpDir, shareID, strconv.Itoa(fileIndex))
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}

	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

func (s *FileStorage) RemoveShareFolder(shareID string) error {
	return os.RemoveAll(filepath.Join(s.tmpDir, shareID))
}

func (s *FileStorage) WriteChunk(shareID string, fileIndex int, chunkIndex int, chunkReader io.Reader) error {
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
