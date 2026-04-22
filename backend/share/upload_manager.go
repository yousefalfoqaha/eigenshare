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
	ResultChan chan<- ChunkResult
}

func (e *UploadChunkEvent) Fail(err error) {
	e.ResultChan <- ChunkResult{IsComplete: false, Err: err}
}

func (e *UploadChunkEvent) Success(isComplete bool) {
	e.ResultChan <- ChunkResult{IsComplete: isComplete, Err: nil}
}

type RegisterShareEvent struct {
	Share Share
}

type UploadManager struct {
	uploadChunkEvents   chan UploadChunkEvent
	registerShareEvents chan RegisterShareEvent
	uploading           map[FileKey]*FileUploadState
}

func NewUploadManager() *UploadManager {
	m := &UploadManager{
		uploadChunkEvents:   make(chan UploadChunkEvent, 1000),
		registerShareEvents: make(chan RegisterShareEvent, 100),
		uploading:           make(map[FileKey]*FileUploadState),
	}

	go m.run()

	return m
}

func (m *UploadManager) run() {
	for {
		select {
		case reg := <-m.registerShareEvents:
			m.handleRegisterShare(reg)
		case e := <-m.uploadChunkEvents:
			m.handleUploadChunk(e)
		}
	}
}

func (m *UploadManager) TrackShareUpload(share Share) {
	m.registerShareEvents <- RegisterShareEvent{Share: share}
}

func (m *UploadManager) UploadChunk(shareID string, fileIndex int, chunkIndex int) {
	key := FileKey{ShareID: shareID, FileIndex: fileIndex}
	m.uploadChunkEvents <- UploadChunkEvent{FileKey: key, ChunkIndex: chunkIndex}
}

func (m *UploadManager) handleRegisterShare(reg RegisterShareEvent) {
	for _, f := range reg.Share.Files {
		numChunks := (f.Size + CHUNK_SIZE - 1) / CHUNK_SIZE
		numBytes := (numChunks + 7) / 8

		key := FileKey{ShareID: reg.Share.ID, FileIndex: f.Index}
		m.uploading[key] = &FileUploadState{
			Bitmask:     make(Bitset, numBytes),
			TotalChunks: int(numChunks),
		}
	}
}

func (m *UploadManager) handleUploadChunk(e UploadChunkEvent) {
	state, ok := m.uploading[e.FileKey]
	if !ok {
		e.Fail(fmt.Errorf("upload state for share %s, file %d not found", e.FileKey.ShareID, e.FileKey.FileIndex))
		return
	}
	if e.ChunkIndex >= state.TotalChunks || e.ChunkIndex < 0 {
		e.Fail(fmt.Errorf("chunk index %d out of bounds", e.ChunkIndex))
		return
	}
	if state.Bitmask.IsSet(e.ChunkIndex) {
		e.Fail(fmt.Errorf("chunk %d was already uploaded", e.ChunkIndex))
		return
	}

	state.Bitmask.Set(e.ChunkIndex)
	state.ChunksUploaded++

	isComplete := state.ChunksUploaded == state.TotalChunks

	if isComplete {
		delete(m.uploading, e.FileKey)
	}

	e.Success(isComplete)
}
