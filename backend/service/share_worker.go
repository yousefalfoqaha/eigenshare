package service

import (
	"context"
	"eigenshare/backend/repository"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
	"errors"
	"fmt"
	"log"
	"time"
)

var (
	ErrShareNotComplete = errors.New("share upload is not complete")
	ErrShareNotTracked  = errors.New("share is not tracked")
)

type updateShareStatusEvent struct {
	ShareID string
	Status  string
	Err     chan error
}

type UploadWorker struct {
	interval        time.Duration
	shareRepository *repository.ShareRepository
	uploadTracker   *upload.UploadTracker
	fileStorage     *storage.FileStorage

	updateShareStatusEvents chan updateShareStatusEvent
}

func NewUploadWorker(
	interval time.Duration,
	shareRepository *repository.ShareRepository,
	uploadTracker *upload.UploadTracker,
	fileStorage *storage.FileStorage,
) *UploadWorker {
	w := &UploadWorker{
		interval:                interval,
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

func (w *UploadWorker) completeShare(id string) error {
	status, err := w.uploadTracker.GetUploadStatus(id)
	if err != nil {
		if errors.Is(err, upload.ErrNotTracked) {
			return ErrShareNotTracked
		}
		return err
	}
	if status != share.STATUS_COMPLETE {
		return ErrShareNotComplete
	}

	if err := w.fileStorage.PersistShareFolder(id); err != nil {
		return err
	}

	rowsAffected, err := w.shareRepository.UpdateStatus(id, share.STATUS_COMPLETE, share.STATUS_UPLOADING)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		log.Printf("share %s already transitioned to COMPLETE", id)
	}

	w.uploadTracker.UntrackUploadProgress(id)

	return nil
}

func (w *UploadWorker) staleShare(id string) error {
	status, err := w.uploadTracker.GetUploadStatus(id)
	if err != nil {
		if errors.Is(err, upload.ErrNotTracked) {
			return nil
		}
		return err
	}
	if status != share.STATUS_PENDING {
		return nil
	}

	rowsAffected, err := w.shareRepository.UpdateStatus(id, share.STATUS_PENDING, share.STATUS_UPLOADING)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return nil
	}

	return nil
}

func (w *UploadWorker) resumeShare(id string) error {
	rowsAffected, err := w.shareRepository.UpdateStatus(id, share.STATUS_UPLOADING, share.STATUS_PENDING)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("share %s is no longer in a pending state", id)
	}

	sh, err := w.shareRepository.Get(id)
	if err != nil {
		return err
	}

	return w.uploadTracker.TrackUploadProgress(*sh)
}

func (w *UploadWorker) scan() {
	uploadingIDs, err := w.shareRepository.GetAllUploadingShares()
	if err != nil {
		log.Printf("error fetching uploading shares: %v", err)
		return
	}

	for _, id := range uploadingIDs {
		status, err := w.uploadTracker.GetUploadStatus(id)
		if err != nil {
			if errors.Is(err, upload.ErrNotTracked) {
				sh, getErr := w.shareRepository.Get(id)
				if getErr != nil {
					log.Printf("error fetching share %s: %v", id, getErr)
					continue
				}
				if trackErr := w.uploadTracker.TrackUploadProgress(*sh); trackErr != nil {
					log.Printf("error re-tracking share %s: %v", id, trackErr)
				}
				continue
			}
			log.Printf("error getting status for %s: %v", id, err)
			continue
		}

		switch status {
		case share.STATUS_COMPLETE:
			w.updateShareStatusEvents <- updateShareStatusEvent{ShareID: id, Status: share.STATUS_COMPLETE}
		case share.STATUS_PENDING:
			w.updateShareStatusEvents <- updateShareStatusEvent{ShareID: id, Status: share.STATUS_PENDING}
		}
	}
}
