package service

import (
	"context"
	"eigenshare/backend/repository"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
	"log"
	"time"
)

type completeShareEvent struct {
	ShareID string
	Err     chan error
}

type UploadWorker struct {
	interval        int
	shareRepository *repository.ShareRepository
	uploadTracker   *upload.UploadTracker
	fileStorage     *storage.FileStorage

	completeShareEvents chan completeShareEvent
}

func NewUploadWorker(
	ctx context.Context,
	interval int,
	shareRepository *repository.ShareRepository,
	uploadTracker *upload.UploadTracker,
	fileStorage *storage.FileStorage,
) *UploadWorker {
	w := &UploadWorker{
		interval:        interval,
		shareRepository: shareRepository,
		uploadTracker:   uploadTracker,
		fileStorage:     fileStorage,

		completeShareEvents: make(chan completeShareEvent, 10),
	}
	go w.run(ctx)

	return w
}

func (w *UploadWorker) run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(w.interval) * time.Millisecond)
	uploadingIDs, err := w.shareRepository.GetAllUploadingShares()
	if err != nil {
		log.Printf("%v", err)
	}

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case e := <-w.completeShareEvents:
			err := w.completeShare(e.ShareID)
			if err != nil {
				e.Err <- err
			}
		case <-ticker.C:
			w.scanUploadingShares(uploadingIDs)
		}
	}
}

func (w *UploadWorker) completeShare(id string) error {
	err := w.shareRepository.UpdateStatus(id, share.STATUS_COMPLETE)
	if err != nil {
		return err
	}

	return nil
}

func (w *UploadWorker) scanUploadingShares(uploadingIDs []string) error {
	for _, id := range uploadingIDs {
		progress := w.uploadTracker.GetShareUploadStatus(id)
		isStale := time.Since(progress.LastActivityAt) > 5*time.Minute
		isComplete := w.shareIsComplete(progress)

		if isComplete {
			w.completeShareEvents <- completeShareEvent{ShareID: id}
		}
	}
}

func (w *UploadWorker) shareIsComplete(progress upload.ShareUploadStatus) bool {
	var markedChunks int
	var expectedChunks int
	for _, f := range progress.Files {
		markedChunks += f.MarkedChunks
		expectedChunks += f.ExpectedChunks
	}

	return markedChunks == expectedChunks
}
