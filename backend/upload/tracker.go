package upload

import (
	"eigenshare/backend/share"
	"errors"
	"fmt"
	"sync"
	"time"
)

type internalFileUploadProgress struct {
	FileIndex      int
	Bitmask        Bitset
	ExpectedChunks int
	ChunksUploaded int
}

type internalUploadProgress struct {
	Files          []internalFileUploadProgress
	LastActivityAt time.Time
}

type UploadTracker struct {
	mu sync.Mutex

	uploading    map[string]*internalUploadProgress
	chunkSize    int64
	staleTimeout time.Duration
}

func NewUploadTracker(chunkSize int64, staleTimeout time.Duration) *UploadTracker {
	return &UploadTracker{
		uploading:    make(map[string]*internalUploadProgress),
		chunkSize:    chunkSize,
		staleTimeout: staleTimeout,
	}
}

func (m *UploadTracker) TrackUploadProgress(sh share.Share) error {
	for i, f := range sh.Files {
		if f.Index != i {
			return fmt.Errorf("file index %d out of order: expected %d", f.Index, i)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.uploading[sh.ID]; exists {
		return nil
	}

	files := make([]internalFileUploadProgress, len(sh.Files))
	for i := range sh.Files {
		numChunks := (sh.Files[i].Size + m.chunkSize - 1) / m.chunkSize
		numBytes := (numChunks + 7) / 8
		files[i] = internalFileUploadProgress{
			FileIndex:      sh.Files[i].Index,
			Bitmask:        make(Bitset, numBytes),
			ExpectedChunks: int(numChunks),
		}
	}

	m.uploading[sh.ID] = &internalUploadProgress{
		Files:          files,
		LastActivityAt: time.Now(),
	}

	return nil
}

func (m *UploadTracker) UntrackUploadProgress(shareID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.uploading, shareID)
}

func (m *UploadTracker) MarkChunk(shareID string, fileIndex int, chunkIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.uploading[shareID]
	if !ok {
		return
	}
	if fileIndex < 0 || fileIndex >= len(state.Files) {
		return
	}

	fs := &state.Files[fileIndex]
	if chunkIndex < 0 || chunkIndex >= fs.ExpectedChunks {
		return
	}

	if !fs.Bitmask.IsSet(chunkIndex) {
		fs.Bitmask.Set(chunkIndex)
		fs.ChunksUploaded++
		state.LastActivityAt = time.Now()
	}
}

func (m *UploadTracker) GetUploadProgress(shareID string) UploadProgress {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.uploading[shareID]
	if !ok {
		return UploadProgress{}
	}

	fileStatuses := make([]FileUploadStatus, len(state.Files))
	for i, f := range state.Files {
		fileStatuses[i] = FileUploadStatus{
			FileIndex:      f.FileIndex,
			ExpectedChunks: f.ExpectedChunks,
			MarkedChunks:   f.ChunksUploaded,
		}
	}

	return UploadProgress{Files: fileStatuses, LastActivityAt: state.LastActivityAt}
}

var ErrNotTracked = errors.New("Share is not tracked")

func (m *UploadTracker) GetUploadStatus(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.uploading[id]
	if !ok {
		return "", ErrNotTracked
	}

	cutoff := time.Now().Add(-m.staleTimeout)
	if state.LastActivityAt.Before(cutoff) {
		return share.STATUS_PENDING, nil
	}

	for _, f := range state.Files {
		if f.ChunksUploaded != f.ExpectedChunks {
			return share.STATUS_UPLOADING, nil
		}
	}

	return share.STATUS_COMPLETE, nil
}
