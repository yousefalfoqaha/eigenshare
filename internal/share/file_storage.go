package share

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type FileStorage struct {
	dataDir string
	locks   sync.Map
}

func NewFileStorage(dataDir string) *FileStorage {
	return &FileStorage{dataDir: dataDir}
}

func (s *FileStorage) lockFor(fileID string) *sync.Mutex {
	l, _ := s.locks.LoadOrStore(fileID, &sync.Mutex{})
	return l.(*sync.Mutex)
}

func (s *FileStorage) FileUploadState(shareID, fileID string) (FileUploadState, error) {
	info, err := os.Stat(filepath.Join(s.dataDir, shareID, fileID))
	if os.IsNotExist(err) {
		return FileUploadState{}, nil
	}
	if err != nil {
		return FileUploadState{}, err
	}

	lock := s.lockFor(fileID)
	locked := !lock.TryLock()
	if !locked {
		lock.Unlock()
	}

	return FileUploadState{
		Written: info.Size(),
		Locked:  locked,
	}, nil
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
	lock := s.lockFor(fileID)
	if !lock.TryLock() {
		return 0, ErrUploadInProgress
	}
	defer lock.Unlock()

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
		return stat.Size(), ErrInvalidChunkOffset
	}

	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}

	remaining := expectedSize - offset
	written, err := io.Copy(f, io.LimitReader(fileReader, remaining))
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return offset + written, nil
		}

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

		if !f.Complete(stat.Size()) {
			return false, nil
		}
	}

	return true, nil
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

func (s *FileStorage) GetKeys() (map[string]struct{}, error) {
	keys := make(map[string]struct{})

	entries, err := os.ReadDir(s.dataDir)
	if os.IsNotExist(err) {
		return keys, nil
	}
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		shareID := entry.Name()
		keys[shareID] = struct{}{}

		fileEntries, err := os.ReadDir(filepath.Join(s.dataDir, shareID))
		if err != nil {
			continue
		}

		for _, fe := range fileEntries {
			if !fe.IsDir() {
				keys[shareID+"/"+fe.Name()] = struct{}{}
			}
		}
	}

	return keys, nil
}
