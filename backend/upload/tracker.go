package upload

import (
	"eigenshare/backend/share"
	"fmt"
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

type markChunkResult struct {
	MarkedShareChunks   int
	ExpectedShareChunks int
	Err                 error
}

type markChunkEvent struct {
	ShareID    string
	FileIndex  int
	ChunkIndex int
}

type trackShareEvent struct {
	Share share.Share
	Err   chan error
}

type untrackShareEvent struct {
	ShareID string
	Err     chan error
}

type getUploadStatusEvent struct {
	ShareID string
	Status  chan ShareUploadStatus
}

type UploadTracker struct {
	markChunkEvents       chan markChunkEvent
	trackShareEvents      chan trackShareEvent
	untrackShareEvents    chan untrackShareEvent
	getUploadStatusEvents chan getUploadStatusEvent
	uploading             map[string]*shareUploadState
	chunkSize             int64
}

func NewUploadTracker(chunkSize int64) *UploadTracker {
	m := &UploadTracker{
		markChunkEvents:       make(chan markChunkEvent, 1000),
		trackShareEvents:      make(chan trackShareEvent, 100),
		untrackShareEvents:    make(chan untrackShareEvent, 100),
		getUploadStatusEvents: make(chan getUploadStatusEvent, 100),
		uploading:             make(map[string]*shareUploadState),
		chunkSize:             chunkSize,
	}
	go m.run()

	return m
}

func (m *UploadTracker) run() {
	for {
		select {
		case reg := <-m.trackShareEvents:
			reg.Err <- m.handleTrackShare(reg)
		case unreg := <-m.untrackShareEvents:
			unreg.Err <- m.handleUntrackShare(unreg)
		case e := <-m.markChunkEvents:
			m.handleUploadChunk(e)
		case e := <-m.getUploadStatusEvents:
			e.Status <- m.handleGetUploadStatus(e.ShareID)
		}
	}
}

func (m *UploadTracker) TrackShareUpload(sh share.Share) error {
	err := make(chan error, 1)
	m.trackShareEvents <- trackShareEvent{Share: sh, Err: err}
	return <-err
}

func (m *UploadTracker) UntrackShareUpload(shareID string) {
	err := make(chan error, 1)
	m.untrackShareEvents <- untrackShareEvent{ShareID: shareID, Err: err}
	<-err
}

func (m *UploadTracker) MarkChunk(shareID string, fileIndex int, chunkIndex int) {
	m.markChunkEvents <- markChunkEvent{
		ShareID:    shareID,
		FileIndex:  fileIndex,
		ChunkIndex: chunkIndex,
	}
}

func (m *UploadTracker) GetShareUploadStatus(shareID string) ShareUploadStatus {
	status := make(chan ShareUploadStatus, 1)
	m.getUploadStatusEvents <- getUploadStatusEvent{ShareID: shareID, Status: status}
	return <-status
}

func (m *UploadTracker) handleTrackShare(reg trackShareEvent) error {
	files := make([]fileUploadState, len(reg.Share.Files))

	for i := range reg.Share.Files {
		numChunks := (reg.Share.Files[i].Size + m.chunkSize - 1) / m.chunkSize
		numBytes := (numChunks + 7) / 8

		files[reg.Share.Files[i].Index] = fileUploadState{
			FileIndex:      reg.Share.Files[i].Index,
			Bitmask:        make(Bitset, numBytes),
			ExpectedChunks: int(numChunks),
			ChunksUploaded: 0,
		}
	}

	m.uploading[reg.Share.ID] = &shareUploadState{
		Files:          files,
		LastActivityAt: time.Now(),
	}

	return nil
}

func (m *UploadTracker) handleUntrackShare(unreg untrackShareEvent) error {
	delete(m.uploading, unreg.ShareID)
	return nil
}

func (m *UploadTracker) handleUploadChunk(e markChunkEvent) (int, int, error) {
	state, ok := m.uploading[e.ShareID]
	if !ok {
		return 0, 0, fmt.Errorf("upload state for share %s not found", e.ShareID)
	}
	if e.FileIndex < 0 || e.FileIndex >= len(state.Files) {
		return 0, 0, fmt.Errorf("file index %d out of bounds", e.FileIndex)
	}

	fs := &state.Files[e.FileIndex]
	if e.ChunkIndex >= fs.ExpectedChunks || e.ChunkIndex < 0 {
		return 0, 0, fmt.Errorf("chunk index %d out of bounds", e.ChunkIndex)
	}

	if !fs.Bitmask.IsSet(e.ChunkIndex) {
		fs.Bitmask.Set(e.ChunkIndex)
		fs.ChunksUploaded++
		state.LastActivityAt = time.Now()
	}

	var expectedChunks int
	var chunksUploaded int

	for _, f := range state.Files {
		expectedChunks += f.ExpectedChunks
		chunksUploaded += f.ChunksUploaded
	}

	return chunksUploaded, expectedChunks, nil
}

func (m *UploadTracker) handleGetUploadStatus(shareID string) ShareUploadStatus {
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
