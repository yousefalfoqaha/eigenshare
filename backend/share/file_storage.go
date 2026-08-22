package share

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

type FileStorage struct {
	dataDir string
}

func NewFileStorage(dataDir string) *FileStorage {
	return &FileStorage{dataDir}
}

func (s *FileStorage) CreateShareFolder(sh Share) error {
	err := os.MkdirAll(filepath.Join(s.dataDir, sh.ID), 0755)
	if err != nil {
		return err
	}

	for _, f := range sh.Files {
		err := s.createFile(sh.ID, f.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *FileStorage) createFile(shareID string, fileID string) error {
	filePath := filepath.Join(s.dataDir, shareID, fileID)
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return nil
}

func (s *FileStorage) RemoveShareFolder(shareID string) error {
	return os.RemoveAll(filepath.Join(s.dataDir, shareID))
}

func (s *FileStorage) RemoveFile(shareID, fileID string) error {
	err := os.Remove(filepath.Join(s.dataDir, shareID, fileID))
	if os.IsNotExist(err) {
		return ErrFileNotFound
	}
	return err
}

func (s *FileStorage) WriteFileStream(shareID string, fileID string, expectedSize int64, offset int64, fileReader io.Reader) error {
	filePath := filepath.Join(s.dataDir, shareID, fileID)
	f, err := os.OpenFile(filePath, os.O_WRONLY, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if offset != stat.Size() {
		return ErrInvalidChunkOffset
	}

	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	remaining := expectedSize - offset
	if _, err := io.Copy(f, io.LimitReader(fileReader, remaining)); err != nil {
		return err
	}

	return nil
}

func (s *FileStorage) IsShareComplete(sh Share) (bool, error) {
	sharePath := filepath.Join(s.dataDir, sh.ID)

	for _, f := range sh.Files {
		stat, err := os.Stat(filepath.Join(sharePath, f.ID))
		if os.IsNotExist(err) {
			return false, ErrFileNotFound
		}

		inProgress := stat.Size() != f.ExpectedSize
		if inProgress {
			return false, nil
		}
	}

	return true, nil
}

func (s *FileStorage) GetShareBytesWritten(sh Share) (int64, error) {
	sharePath := filepath.Join(s.dataDir, sh.ID)

	var written int64
	for _, f := range sh.Files {
		stat, err := os.Stat(filepath.Join(sharePath, f.ID))
		if os.IsNotExist(err) {
			return 0, ErrFileNotFound
		}

		written += stat.Size()
	}

	return written, nil
}

func (s *FileStorage) ShareFilePath(shareID string, fileID string) (string, error) {
	path := filepath.Join(s.dataDir, shareID, fileID)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", ErrFileNotFound
	}

	return path, nil
}

func (s *FileStorage) CreateShareZipStream(sh Share, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, f := range sh.Files {
		path := filepath.Join(s.dataDir, sh.ID, f.ID)
		src, err := os.Open(path)
		if err != nil {
			return err
		}

		dst, err := zw.Create(f.Name)
		if err != nil {
			src.Close()
			return err
		}

		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			return err
		}

		src.Close()
	}

	return nil
}
