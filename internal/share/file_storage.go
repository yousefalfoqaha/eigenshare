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
	err := os.MkdirAll(filepath.Join(s.dataDir, sh.ID), 0o755)
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

func (s *FileStorage) CreateFile(shareID, fileID string) error {
	if err := os.MkdirAll(filepath.Join(s.dataDir, shareID), 0o755); err != nil {
		return err
	}

	return s.createFile(shareID, fileID)
}

func (s *FileStorage) createFile(shareID string, fileID string) error {
	filePath := filepath.Join(s.dataDir, shareID, fileID)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	return nil
}

func (s *FileStorage) HasIncompleteFiles(sh Share) (bool, error) {
	for _, file := range sh.Files {
		stat, err := os.Stat(filepath.Join(s.dataDir, sh.ID, file.ID))
		if os.IsNotExist(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if stat.Size() != file.ExpectedSize {
			return true, nil
		}
	}

	return false, nil
}

func (s *FileStorage) RemoveShareFolder(ids ...string) error {
	for _, id := range ids {
		err := os.RemoveAll(filepath.Join(s.dataDir, id))
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *FileStorage) GetShareIDs() ([]string, error) {
	entries, err := os.ReadDir(s.dataDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var shareIDs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		shareIDs = append(shareIDs, entry.Name())
	}

	return shareIDs, nil
}

func (s *FileStorage) RemoveFile(shareID, fileID string) error {
	err := os.Remove(filepath.Join(s.dataDir, shareID, fileID))
	if os.IsNotExist(err) {
		return ErrFileNotFound
	}
	return err
}

func (s *FileStorage) WriteFileStream(shareID string, fileID string, expectedSize int64, offset int64, fileReader io.Reader) (int64, error) {
	if err := os.MkdirAll(filepath.Join(s.dataDir, shareID), 0o755); err != nil {
		return 0, err
	}

	filePath := filepath.Join(s.dataDir, shareID, fileID)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}

	if offset != stat.Size() {
		return 0, &ChunkOffsetConflictError{
			CurrentOffset: stat.Size(),
			ExpectedSize:  expectedSize,
		}
	}

	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}

	remaining := expectedSize - offset
	written, err := io.Copy(f, io.LimitReader(fileReader, remaining))
	if err != nil {
		return 0, err
	}

	return offset + written, nil
}

func (s *FileStorage) IsShareComplete(sh Share) (bool, error) {
	sharePath := filepath.Join(s.dataDir, sh.ID)

	for _, f := range sh.Files {
		stat, err := os.Stat(filepath.Join(sharePath, f.ID))
		if os.IsNotExist(err) {
			return false, nil
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
			continue
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
