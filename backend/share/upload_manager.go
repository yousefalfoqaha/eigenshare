package share

import "fmt"

type Bitset []byte

func (b Bitset) Set(index int) {
	b[index/8] |= (1 << (index % 8))
}

func (b Bitset) IsSet(index int) bool {
	if index < 0 || index/8 >= len(b) {
		return false
	}

	return b[index/8]&(1<<(index%8)) != 0
}

type FileKey struct {
	ShareID   string
	FileIndex int
}

type FileUploadState struct {
	Bitmask        Bitset
	TotalChunks    int
	ChunksUploaded int
}

type ChunkResult struct {
	IsComplete bool
	Err        error
}

type UploadChunkEvent struct {
	FileKey    FileKey
	ChunkIndex int
	ResultChan chan ChunkResult
}

func (e *UploadChunkEvent) Fail(err error) {
	e.ResultChan <- ChunkResult{IsComplete: false, Err: err}
}

func (e *UploadChunkEvent) Success(isComplete bool) {
	e.ResultChan <- ChunkResult{IsComplete: isComplete, Err: nil}
}

type RegisterResult struct {
	Err error
}

type RegisterShareEvent struct {
	Share      Share
	ResultChan chan RegisterResult
}

type UnregisterShareEvent struct {
	ShareID    string
	ResultChan chan RegisterResult
}

type UploadManager struct {
	uploadChunkEvents     chan UploadChunkEvent
	registerShareEvents   chan RegisterShareEvent
	unregisterShareEvents chan UnregisterShareEvent
	uploading             map[FileKey]*FileUploadState
}

func NewUploadManager() *UploadManager {
	m := &UploadManager{
		uploadChunkEvents:     make(chan UploadChunkEvent, 1000),
		registerShareEvents:   make(chan RegisterShareEvent, 100),
		unregisterShareEvents: make(chan UnregisterShareEvent, 100),
		uploading:             make(map[FileKey]*FileUploadState),
	}

	go m.run()

	return m
}

func (m *UploadManager) run() {
	for {
		select {
		case reg := <-m.registerShareEvents:
			m.handleRegisterShare(reg)
		case unreg := <-m.unregisterShareEvents:
			m.handleUnregisterShare(unreg)
		case e := <-m.uploadChunkEvents:
			isComplete, err := m.handleUploadChunk(e)
			if err != nil {
				e.Fail(err)
				continue
			}

			e.Success(isComplete)
		}
	}
}

func (m *UploadManager) TrackShareUpload(share Share) error {
	resultChan := make(chan RegisterResult, 1)
	m.registerShareEvents <- RegisterShareEvent{Share: share, ResultChan: resultChan}
	return (<-resultChan).Err
}

func (m *UploadManager) UntrackShareUpload(shareID string) {
	resultChan := make(chan RegisterResult, 1)
	m.unregisterShareEvents <- UnregisterShareEvent{ShareID: shareID, ResultChan: resultChan}
	<-resultChan
}

func (m *UploadManager) UploadChunk(shareID string, fileIndex int, chunkIndex int) (bool, error) {
	key := FileKey{ShareID: shareID, FileIndex: fileIndex}
	resultChan := make(chan ChunkResult, 1)

	m.uploadChunkEvents <- UploadChunkEvent{FileKey: key, ChunkIndex: chunkIndex, ResultChan: resultChan}
	result := <-resultChan

	return result.IsComplete, result.Err
}

func (m *UploadManager) handleRegisterShare(reg RegisterShareEvent) {
	for _, f := range reg.Share.Files {
		numChunks := (f.Size + CHUNK_SIZE - 1) / CHUNK_SIZE
		numBytes := (numChunks + 7) / 8

		key := FileKey{ShareID: reg.Share.ID, FileIndex: f.Index}
		m.uploading[key] = &FileUploadState{
			Bitmask:        make(Bitset, numBytes),
			TotalChunks:    int(numChunks),
			ChunksUploaded: 0,
		}
	}

	reg.ResultChan <- RegisterResult{Err: nil}
}

func (m *UploadManager) handleUnregisterShare(unreg UnregisterShareEvent) {
	for key := range m.uploading {
		if key.ShareID == unreg.ShareID {
			delete(m.uploading, key)
		}
	}

	unreg.ResultChan <- RegisterResult{Err: nil}
}

func (m *UploadManager) handleUploadChunk(e UploadChunkEvent) (bool, error) {
	state, ok := m.uploading[e.FileKey]
	if !ok {
		return false, fmt.Errorf("upload state for share %s, file %d not found", e.FileKey.ShareID, e.FileKey.FileIndex)
	}
	if e.ChunkIndex >= state.TotalChunks || e.ChunkIndex < 0 {
		return false, fmt.Errorf("chunk index %d out of bounds", e.ChunkIndex)
	}
	if state.Bitmask.IsSet(e.ChunkIndex) {
		return false, fmt.Errorf("chunk %d was already uploaded", e.ChunkIndex)
	}

	state.Bitmask.Set(e.ChunkIndex)
	state.ChunksUploaded++

	isComplete := state.ChunksUploaded == state.TotalChunks

	if isComplete {
		delete(m.uploading, e.FileKey)
	}

	return isComplete, nil
}
