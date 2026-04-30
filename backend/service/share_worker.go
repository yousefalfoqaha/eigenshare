package service

import (
	"eigenshare/backend/repository"
	"eigenshare/backend/storage"
	"eigenshare/backend/upload"
	"time"
)

type UploadWorker struct {
	interval        int
	shareRepository *repository.ShareRepository
	uploadManager   *upload.UploadManager
	fileStorage     *storage.FileStorage
}

func NewUploadWorker(
	interval int,
	shareRepository *repository.ShareRepository,
	uploadManager *upload.UploadManager,
	fileStorage *storage.FileStorage,
) *UploadWorker {
	w := &UploadWorker{interval, shareRepository, uploadManager, fileStorage}
	go w.run()

	return w
}

func (w *UploadWorker) run() {
	for {
		w.scanStaleUploads()
		time.Sleep(time.Duration(w.interval) * time.Second)
	}
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
