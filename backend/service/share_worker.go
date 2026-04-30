package service

import (
	"context"
	"eigenshare/backend/repository"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
	"time"
)

type completeEvent struct {
	ShareID string
	Err     chan error
}

type UploadWorker struct {
	interval        int
	shareRepository *repository.ShareRepository
	uploadManager   *upload.UploadManager
	fileStorage     *storage.FileStorage

	completeEvents chan completeEvent
}

func NewUploadWorker(
	ctx context.Context,
	interval int,
	shareRepository *repository.ShareRepository,
	uploadManager *upload.UploadManager,
	fileStorage *storage.FileStorage,
) *UploadWorker {
	w := &UploadWorker{
		interval:        interval,
		shareRepository: shareRepository,
		uploadManager:   uploadManager,
		fileStorage:     fileStorage,

		completeEvents: make(chan completeEvent, 10),
	}
	go w.run(ctx)

	return w
}

func (w *UploadWorker) run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(w.interval) * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case e := <-w.completeEvents:
			err := w.completeShare(e.ShareID)
			if err != nil {
				e.Err <- err
			}
		}
	}
}

func (w *UploadWorker) completeShare(shareID string) error {
	// TODO
	return nil
}

func (w *UploadWorker) scanStaleUploads() error {
	uploadingIDs, err := w.shareRepository.GetAllUploadingShares()
	if err != nil {
		return err
	}

	for _, id := range uploadingIDs {
		persisted, err := w.fileStorage.IsSharePersisted(id)
		if err != nil {
			return err
		}

	}
}
