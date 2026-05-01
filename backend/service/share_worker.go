package service

import (
	"context"
	"eigenshare/backend/repository"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
	"fmt"
	"log"
	"time"
)

type updateShareStatusEvent struct {
	ShareID string
	Status  string
	Err     chan error
}

type UploadWorker struct {
	interval        time.Duration
	staleTimeout    time.Duration
	shareRepository *repository.ShareRepository
	uploadTracker   *upload.UploadTracker
	fileStorage     *storage.FileStorage

	updateShareStatusEvents chan updateShareStatusEvent
}

func NewUploadWorker(
	interval time.Duration,
	staleTimeout time.Duration,
	shareRepository *repository.ShareRepository,
	uploadTracker *upload.UploadTracker,
	fileStorage *storage.FileStorage,
) *UploadWorker {
	w := &UploadWorker{
		interval:                interval,
		staleTimeout:            staleTimeout,
		shareRepository:         shareRepository,
		uploadTracker:           uploadTracker,
		fileStorage:             fileStorage,
		updateShareStatusEvents: make(chan updateShareStatusEvent, 10),
	}

	return w
}

func (w *UploadWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case e := <-w.updateShareStatusEvents:
			err := w.handleUpdateShareStatus(e.ShareID, e.Status)
			if e.Err != nil {
				e.Err <- err
			}
		case <-ticker.C:
			go w.scan()
		}
	}
}

func (w *UploadWorker) RequestStatusUpdate(shareID string, status string) error {
	err := make(chan error, 1)
	w.updateShareStatusEvents <- updateShareStatusEvent{ShareID: shareID, Status: status, Err: err}
	return <-err
}

func (w *UploadWorker) handleUpdateShareStatus(shareID string, status string) error {
	switch status {
	case share.STATUS_COMPLETE:
		return w.completeShare(shareID)
	case share.STATUS_PENDING:
		return w.staleShare(shareID)
	case share.STATUS_UPLOADING:
		return w.resumeShare(shareID)
	default:
		log.Printf("unknown status transition: %s -> %s", shareID, status)
		return nil
	}
}

func (w *UploadWorker) completeShare(shareID string) error {
	persisted, err := w.fileStorage.IsSharePersisted(shareID)
	if err != nil {
		return err
	}

	if !persisted {
		if err := w.fileStorage.PersistShareFolder(shareID); err != nil {
			return err
		}
	}

	rowsAffected, err := w.shareRepository.UpdateStatus(shareID, share.STATUS_COMPLETE, share.STATUS_UPLOADING)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		log.Printf("share %s already transitioned to COMPLETE", shareID)
	}

	w.uploadTracker.UntrackShareUpload(shareID)

	return nil
}

func (w *UploadWorker) staleShare(shareID string) error {
	rowsAffected, err := w.shareRepository.UpdateStatus(shareID, share.STATUS_PENDING, share.STATUS_UPLOADING)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return nil
	}

	w.uploadTracker.UntrackShareUpload(shareID)

	return nil
}

func (w *UploadWorker) resumeShare(shareID string) error {
	rowsAffected, err := w.shareRepository.UpdateStatus(shareID, share.STATUS_UPLOADING, share.STATUS_PENDING)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("share %s is no longer in pending state", shareID)
	}

	sh, err := w.shareRepository.Get(shareID)
	if err != nil {
		return err
	}

	return w.uploadTracker.TrackShareUpload(*sh)
}

func (w *UploadWorker) scan() {
	trackedIDs := w.uploadTracker.GetTrackedShareIDs()
	staleIDs := w.uploadTracker.GetStaleShareIDs(w.staleTimeout)
	uploadingIDs, err := w.shareRepository.GetAllUploadingShares()
	if err != nil {
		log.Printf("error fetching uploading shares: %v", err)
		return
	}

	for _, id := range trackedIDs {
		if w.uploadTracker.IsShareComplete(id) {
			w.updateShareStatusEvents <- updateShareStatusEvent{ShareID: id, Status: share.STATUS_COMPLETE}
		}
	}

	for _, id := range staleIDs {
		w.updateShareStatusEvents <- updateShareStatusEvent{ShareID: id, Status: share.STATUS_PENDING}
	}

	orphanedIDs := w.uploadTracker.GetOrphanedShareIDs(uploadingIDs)
	for _, id := range orphanedIDs {
		persisted, err := w.fileStorage.IsSharePersisted(id)
		if err != nil {
			log.Printf("error checking if share %s is persisted: %v", id, err)
			continue
		}

		if persisted {
			w.updateShareStatusEvents <- updateShareStatusEvent{ShareID: id, Status: share.STATUS_COMPLETE}
			continue
		}

		sh, err := w.shareRepository.Get(id)
		if err != nil {
			log.Printf("error fetching orphaned share %s: %v", id, err)
			continue
		}

		if err := w.uploadTracker.TrackShareUpload(*sh); err != nil {
			log.Printf("error re-tracking share %s: %v", id, err)
		}
	}
}
