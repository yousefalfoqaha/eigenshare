package upload

import (
	"eigenshare/backend/repository"
	"time"
)

type UploadWorker struct {
	interval        int
	shareRepository *repository.ShareRepository
}

func NewUploadWorker(interval int, shareRepository *repository.ShareRepository) *UploadWorker {
	w := &UploadWorker{interval, shareRepository}
	go w.run()

	return w
}

func (w *UploadWorker) run() {
	for {
		time.Sleep(time.Duration(w.interval) * time.Second)
	}
}

func (w *UploadWorker) checkStaleUploads() {
	// TODO
}
