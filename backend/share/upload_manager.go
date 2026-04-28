package share

import "fmt"

type fileKey struct {
	ShareID   string
	FileIndex int
}

type fileUploadState struct {
	Bitmask        Bitset
	TotalChunks    int
	ChunksUploaded int
}

type chunkUploadResult struct {
	ChunksUploaded int
	TotalChunks    int
	Err            error
}

type fileUploadStatus struct {
	FileIndex      int `json:"fileIndex"`
	ChunksUploaded int `json:"chunksUploaded"`
	TotalChunks    int `json:"totalChunks"`
}

type shareUploadStatus struct {
	Files []fileUploadStatus `json:"files"`
}

type uploadChunkEvent struct {
	FileKey    fileKey
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
	Status  chan shareUploadStatus
}

type UploadManager struct {
	uploadChunkEvents     chan uploadChunkEvent
	registerShareEvents   chan registerShareEvent
	unregisterShareEvents chan unregisterShareEvent
	getUploadStatusEvents chan getUploadStatusEvent
	uploading             map[fileKey]*fileUploadState
	chunkSize             int64
}

func NewUploadManager(chunkSize int64) *UploadManager {
	m := &UploadManager{
		uploadChunkEvents:     make(chan uploadChunkEvent, 1000),
		registerShareEvents:   make(chan registerShareEvent, 100),
		unregisterShareEvents: make(chan unregisterShareEvent, 100),
		getUploadStatusEvents: make(chan getUploadStatusEvent, 100),
		uploading:             make(map[fileKey]*fileUploadState),
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
			e.ResultChan <- chunkUploadResult{ChunksUploaded: chunksUploaded, TotalChunks: totalChunks}
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
	key := fileKey{ShareID: shareID, FileIndex: fileIndex}
	resultChan := make(chan chunkUploadResult, 1)

	m.uploadChunkEvents <- uploadChunkEvent{FileKey: key, ChunkIndex: chunkIndex, ResultChan: resultChan}
	result := <-resultChan

	return result.ChunksUploaded, result.TotalChunks, result.Err
}

func (m *UploadManager) GetShareUploadStatus(shareID string) shareUploadStatus {
	status := make(chan shareUploadStatus, 1)
	m.getUploadStatusEvents <- getUploadStatusEvent{ShareID: shareID, Status: status}
	return <-status
}

func (m *UploadManager) handleRegisterShare(reg registerShareEvent) error {
	for _, f := range reg.Share.Files {
		numChunks := (f.Size + m.chunkSize - 1) / m.chunkSize
		numBytes := (numChunks + 7) / 8

		key := fileKey{ShareID: reg.Share.ID, FileIndex: f.Index}
		m.uploading[key] = &fileUploadState{
			Bitmask:        make(Bitset, numBytes),
			TotalChunks:    int(numChunks),
			ChunksUploaded: 0,
		}
	}

	return nil
}

func (m *UploadManager) handleUnregisterShare(unreg unregisterShareEvent) error {
	for key := range m.uploading {
		if key.ShareID == unreg.ShareID {
			delete(m.uploading, key)
		}
	}

	return nil
}

func (m *UploadManager) handleUploadChunk(e uploadChunkEvent) (int, int, error) {
	state, ok := m.uploading[e.FileKey]
	if !ok {
		return 0, 0, fmt.Errorf("upload state for share %s, file %d not found", e.FileKey.ShareID, e.FileKey.FileIndex)
	}
	if e.ChunkIndex >= state.TotalChunks || e.ChunkIndex < 0 {
		return 0, 0, fmt.Errorf("chunk index %d out of bounds", e.ChunkIndex)
	}
	if state.Bitmask.IsSet(e.ChunkIndex) {
		return 0, 0, fmt.Errorf("chunk %d was already uploaded", e.ChunkIndex)
	}

	state.Bitmask.Set(e.ChunkIndex)
	state.ChunksUploaded++

	if state.ChunksUploaded == state.TotalChunks {
		delete(m.uploading, e.FileKey)
	}

	return state.ChunksUploaded, state.TotalChunks, nil
}

func (m *UploadManager) handleGetUploadStatus(shareID string) shareUploadStatus {
	var statuses []fileUploadStatus

	for key, state := range m.uploading {
		if key.ShareID == shareID {
			statuses = append(statuses, fileUploadStatus{
				FileIndex:      key.FileIndex,
				ChunksUploaded: state.ChunksUploaded,
				TotalChunks:    state.TotalChunks,
			})
		}
	}

	return shareUploadStatus{Files: statuses}
}
