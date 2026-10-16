package share

import (
	"context"
	"log"
	"strings"
	"time"
)

type ShareCleaner struct {
	repo  *ShareRepository
	store *FileStorage
}

func NewShareCleaner(repo *ShareRepository, storage *FileStorage) *ShareCleaner {
	return &ShareCleaner{repo: repo, store: storage}
}

func (e *ShareCleaner) Run(ctx context.Context, interval time.Duration) {
	e.clean()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.clean()
		}
	}
}

func (e *ShareCleaner) clean() {
	if err := e.repo.CleanupStale(); err != nil {
		log.Printf("failed to clean up stale shares and upload requests: %v", err)
	}
	if err := e.CleanupStorage(); err != nil {
		log.Printf("failed to clean up orphaned share folders: %v", err)
	}
}

func (e *ShareCleaner) CleanupStorage() error {
	repoKeys, err := e.repo.GetKeys()
	if err != nil {
		return err
	}

	storageKeys, err := e.store.GetKeys()
	if err != nil {
		return err
	}

	for key := range storageKeys {
		if _, ok := repoKeys[key]; ok {
			continue
		}

		shareID, fileID, isFile := strings.Cut(key, "/")
		if isFile {
			_ = e.store.RemoveFile(shareID, fileID)
		} else {
			_ = e.store.RemoveShareFolder(shareID)
		}
	}

	return nil
}
