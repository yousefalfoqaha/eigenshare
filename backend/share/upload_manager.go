package share

import (
	"fmt"
	"time"
)

type fileUploadState struct {
	FileIndex      int    `json:"fileIndex"`
	Bitmask        Bitset `json:"-"`
	TotalChunks    int    `json:"totalChunks"`
	ChunksUploaded int    `json:"chunksUploaded"`
}

type shareUploadState struct {
	Files          []fileUploadState `json:"files"`
	LastActivityAt time.Time         `json:"-"`
}

type chunkUploadResult struct {
	UploadedShareChunks int
	TotalShareChunks    int
	Err                 error
}

type uploadChunkEvent struct {
	ShareID    string
	FileIndex  int
	ChunkIndex int
	ResultChan chan chunkUploadResult
}

type registerShareEvent struct {
	Share Share
	Err   chan error
}

type unregisterShareEvent struct {
	ShareID string
	Err     chan error
}

type getUploadStatusEvent struct {
	ShareID string
	Status  chan shareUploadState
}

type UploadManager struct {
	uploadChunkEvents     chan uploadChunkEvent
	registerShareEvents   chan registerShareEvent
	unregisterShareEvents chan unregisterShareEvent
	getUploadStatusEvents chan getUploadStatusEvent
	uploading             map[string]*shareUploadState
	chunkSize             int64
}

func NewUploadManager(chunkSize int64) *UploadManager {
	m := &UploadManager{
		uploadChunkEvents:     make(chan uploadChunkEvent, 1000),
		registerShareEvents:   make(chan registerShareEvent, 100),
		unregisterShareEvents: make(chan unregisterShareEvent, 100),
		getUploadStatusEvents: make(chan getUploadStatusEvent, 100),
		uploading:             make(map[string]*shareUploadState),
		chunkSize:             chunkSize,
	}

	go m.run()

	return m
}

func (m *UploadManager) run() {
	for {
		select {
		case reg := <-m.registerShareEvents:
			reg.Err <- m.handleRegisterShare(reg)
		case unreg := <-m.unregisterShareEvents:
			unreg.Err <- m.handleUnregisterShare(unreg)
		case e := <-m.uploadChunkEvents:
			chunksUploaded, totalChunks, err := m.handleUploadChunk(e)
			if err != nil {
				e.ResultChan <- chunkUploadResult{Err: err}
				continue
			}
			e.ResultChan <- chunkUploadResult{UploadedShareChunks: chunksUploaded, TotalShareChunks: totalChunks}
		case e := <-m.getUploadStatusEvents:
			e.Status <- m.handleGetUploadStatus(e.ShareID)
		}
	}
}

func (m *UploadManager) TrackShareUpload(share Share) error {
	err := make(chan error, 1)
	m.registerShareEvents <- registerShareEvent{Share: share, Err: err}
	return <-err
}

func (m *UploadManager) UntrackShareUpload(shareID string) {
	err := make(chan error, 1)
	m.unregisterShareEvents <- unregisterShareEvent{ShareID: shareID, Err: err}
	<-err
}

func (m *UploadManager) UploadChunk(shareID string, fileIndex int, chunkIndex int) (int, int, error) {
	resultChan := make(chan chunkUploadResult, 1)

	m.uploadChunkEvents <- uploadChunkEvent{
		ShareID:    shareID,
		FileIndex:  fileIndex,
		ChunkIndex: chunkIndex,
		ResultChan: resultChan,
	}
	result := <-resultChan

	return result.UploadedShareChunks, result.TotalShareChunks, result.Err
}

func (m *UploadManager) GetShareUploadStatus(shareID string) shareUploadState {
	status := make(chan shareUploadState, 1)
	m.getUploadStatusEvents <- getUploadStatusEvent{ShareID: shareID, Status: status}
	return <-status
}

func (m *UploadManager) handleRegisterShare(reg registerShareEvent) error {
	files := make([]fileUploadState, len(reg.Share.Files))

	for i := range reg.Share.Files {
		numChunks := (reg.Share.Files[i].Size + m.chunkSize - 1) / m.chunkSize
		numBytes := (numChunks + 7) / 8

		files[reg.Share.Files[i].Index] = fileUploadState{
			FileIndex:      reg.Share.Files[i].Index,
			Bitmask:        make(Bitset, numBytes),
			TotalChunks:    int(numChunks),
			ChunksUploaded: 0,
		}
	}

	m.uploading[reg.Share.ID] = &shareUploadState{
		Files:          files,
		LastActivityAt: time.Now(),
	}

	return nil
}

func (m *UploadManager) handleUnregisterShare(unreg unregisterShareEvent) error {
	delete(m.uploading, unreg.ShareID)
	return nil
}

func (m *UploadManager) handleUploadChunk(e uploadChunkEvent) (int, int, error) {
	state, ok := m.uploading[e.ShareID]
	if !ok {
		return 0, 0, fmt.Errorf("upload state for share %s not found", e.ShareID)
	}
	if e.FileIndex < 0 || e.FileIndex >= len(state.Files) {
		return 0, 0, fmt.Errorf("file index %d out of bounds", e.FileIndex)
	}

	fs := &state.Files[e.FileIndex]
	if e.ChunkIndex >= fs.TotalChunks || e.ChunkIndex < 0 {
		return 0, 0, fmt.Errorf("chunk index %d out of bounds", e.ChunkIndex)
	}

	if !fs.Bitmask.IsSet(e.ChunkIndex) {
		fs.Bitmask.Set(e.ChunkIndex)
		fs.ChunksUploaded++
		state.LastActivityAt = time.Now()
	}

	var totalChunks int
	var chunksUploaded int

	for _, f := range state.Files {
		totalChunks += f.TotalChunks
		chunksUploaded += f.ChunksUploaded
	}

	return chunksUploaded, totalChunks, nil
}

func (m *UploadManager) handleGetUploadStatus(shareID string) shareUploadState {
	state, ok := m.uploading[shareID]
	if !ok {
		return shareUploadState{}
	}

	return *state
}
