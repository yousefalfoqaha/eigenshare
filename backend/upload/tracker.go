package upload

import (
	"eigenshare/backend/share"
	"fmt"
	"sync"
	"time"
)

type fileUploadState struct {
	FileIndex      int
	Bitmask        Bitset
	ExpectedChunks int
	ChunksUploaded int
}

type shareUploadState struct {
	Files          []fileUploadState
	LastActivityAt time.Time
}

type UploadTracker struct {
	mu        sync.Mutex
	uploading map[string]*shareUploadState
	chunkSize int64
}

func NewUploadTracker(chunkSize int64) *UploadTracker {
	return &UploadTracker{
		uploading: make(map[string]*shareUploadState),
		chunkSize: chunkSize,
	}
}

func (m *UploadTracker) TrackShareUpload(sh share.Share) error {
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

	files := make([]fileUploadState, len(sh.Files))
	for i := range sh.Files {
		numChunks := (sh.Files[i].Size + m.chunkSize - 1) / m.chunkSize
		numBytes := (numChunks + 7) / 8
		files[i] = fileUploadState{
			FileIndex:      sh.Files[i].Index,
			Bitmask:        make(Bitset, numBytes),
			ExpectedChunks: int(numChunks),
		}
	}

	m.uploading[sh.ID] = &shareUploadState{
		Files:          files,
		LastActivityAt: time.Now(),
	}

	return nil
}

func (m *UploadTracker) UntrackShareUpload(shareID string) {
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

func (m *UploadTracker) GetShareUploadStatus(shareID string) ShareUploadStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.uploading[shareID]
	if !ok {
		return ShareUploadStatus{}
	}

	fileStatuses := make([]FileUploadStatus, len(state.Files))
	for i, f := range state.Files {
		fileStatuses[i] = FileUploadStatus{
			FileIndex:      f.FileIndex,
			ExpectedChunks: f.ExpectedChunks,
			MarkedChunks:   f.ChunksUploaded,
		}
	}

	return ShareUploadStatus{Files: fileStatuses, LastActivityAt: state.LastActivityAt}
}

func (m *UploadTracker) IsShareComplete(shareID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.uploading[shareID]
	if !ok {
		return false
	}
	for _, f := range state.Files {
		if f.ChunksUploaded != f.ExpectedChunks {
			return false
		}
	}

	return true
}

func (m *UploadTracker) GetTrackedShareIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	ids := make([]string, 0, len(m.uploading))
	for id := range m.uploading {
		ids = append(ids, id)
	}

	return ids
}

func (m *UploadTracker) GetStaleShareIDs(since time.Duration) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-since)
	var stale []string
	for id, state := range m.uploading {
		if state.LastActivityAt.Before(cutoff) {
			stale = append(stale, id)
		}
	}
	return stale
}

func (m *UploadTracker) GetOrphanedShareIDs(knownIDs []string) []string {
	knownSet := make(map[string]bool, len(knownIDs))
	for _, id := range knownIDs {
		knownSet[id] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var orphans []string
	for id := range m.uploading {
		if !knownSet[id] {
			orphans = append(orphans, id)
		}
	}
	return orphans
}
