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

type FileStatus struct {
	ChunksUploaded int
	TotalChunks    int
	Err            error
}

type UploadChunkEvent struct {
	FileKey    FileKey
	ChunkIndex int
	FileStatus chan FileStatus
}

func (e *UploadChunkEvent) Fail(err error) {
	e.FileStatus <- FileStatus{Err: err}
}

func (e *UploadChunkEvent) SendResult(chunksUploaded, totalChunks int) {
	e.FileStatus <- FileStatus{ChunksUploaded: chunksUploaded, TotalChunks: totalChunks}
}

type RegisterShareEvent struct {
	Share Share
	Err   chan error
}

type UnregisterShareEvent struct {
	ShareID string
	Err     chan error
}

type UploadManager struct {
	uploadChunkEvents     chan UploadChunkEvent
	registerShareEvents   chan RegisterShareEvent
	unregisterShareEvents chan UnregisterShareEvent
	getUploadStatusEvents chan GetUploadStatusEvent
	uploading             map[FileKey]*FileUploadState
}

func NewUploadManager() *UploadManager {
	m := &UploadManager{
		uploadChunkEvents:     make(chan UploadChunkEvent, 1000),
		registerShareEvents:   make(chan RegisterShareEvent, 100),
		unregisterShareEvents: make(chan UnregisterShareEvent, 100),
		getUploadStatusEvents: make(chan GetUploadStatusEvent, 100),
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
			m.handleUploadChunk(e)
		case e := <-m.getUploadStatusEvents:
			m.handleGetUploadStatus(e)
		}
	}
}

func (m *UploadManager) TrackShareUpload(share Share) error {
	err := make(chan error, 1)
	m.registerShareEvents <- RegisterShareEvent{Share: share, Err: err}
	return <-err
}

func (m *UploadManager) UntrackShareUpload(shareID string) {
	err := make(chan error, 1)
	m.unregisterShareEvents <- UnregisterShareEvent{ShareID: shareID, Err: err}
	<-err
}

func (m *UploadManager) UploadChunk(shareID string, fileIndex int, chunkIndex int) (int, int, error) {
	key := FileKey{ShareID: shareID, FileIndex: fileIndex}
	fileStatus := make(chan FileStatus, 1)

	m.uploadChunkEvents <- UploadChunkEvent{FileKey: key, ChunkIndex: chunkIndex, FileStatus: fileStatus}
	result := <-fileStatus

	return result.ChunksUploaded, result.TotalChunks, result.Err
}

type FileUploadStatus struct {
	FileIndex      int
	ChunksUploaded int
	TotalChunks    int
}

type ShareUploadStatus struct {
	Files []FileUploadStatus
}

type GetUploadStatusEvent struct {
	ShareID string
	Status  chan ShareUploadStatus
}

func (m *UploadManager) GetShareUploadStatus(shareID string) ShareUploadStatus {
	status := make(chan ShareUploadStatus, 1)
	m.getUploadStatusEvents <- GetUploadStatusEvent{ShareID: shareID, Status: status}
	return <-status
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

	reg.Err <- nil
}

func (m *UploadManager) handleUnregisterShare(unreg UnregisterShareEvent) {
	for key := range m.uploading {
		if key.ShareID == unreg.ShareID {
			delete(m.uploading, key)
		}
	}

	unreg.Err <- nil
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

	if state.ChunksUploaded == state.TotalChunks {
		delete(m.uploading, e.FileKey)
	}

	e.SendResult(state.ChunksUploaded, state.TotalChunks)
}

func (m *UploadManager) handleGetUploadStatus(e GetUploadStatusEvent) {
	var statuses []FileUploadStatus

	for key, state := range m.uploading {
		if key.ShareID == e.ShareID {
			statuses = append(statuses, FileUploadStatus{
				FileIndex:      key.FileIndex,
				ChunksUploaded: state.ChunksUploaded,
				TotalChunks:    state.TotalChunks,
			})
		}
	}

	e.Status <- ShareUploadStatus{Files: statuses}
}
