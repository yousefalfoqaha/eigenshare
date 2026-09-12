package share

import (
	"context"
	"log"
	"strings"
	"time"

	"eigenshare/internal/timestamp"
)

type Cleaner struct {
	repo    *ShareRepository
	storage *FileStorage
}

func NewCleaner(repo *ShareRepository, storage *FileStorage) *Cleaner {
	return &Cleaner{repo: repo, storage: storage}
}

func (e *Cleaner) Run(ctx context.Context, interval time.Duration) {
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

func (e *Cleaner) clean() {
	if err := e.handleExpired(); err != nil {
		log.Printf("failed to clean up expired shares: %v", err)
	}
	if err := e.handleTrashed(); err != nil {
		log.Printf("failed to clean up trashed share folders: %v", err)
	}
	if err := e.deleteOrphaned(); err != nil {
		log.Printf("failed to clean up orphaned share folders: %v", err)
	}
}

func (e *Cleaner) handleExpired() error {
	shares, err := e.repo.GetAll()
	if err != nil {
		return err
	}

	var expiredIDs []string
	now := timestamp.Now().Time()

	for _, s := range shares {
		if s.ExpiresAt != nil && s.ExpiresAt.Time().Before(now) {
			expiredIDs = append(expiredIDs, s.ID)
		}
	}

	if len(expiredIDs) == 0 {
		return nil
	}

	return e.repo.Delete(expiredIDs...)
}

func (e *Cleaner) handleTrashed() error {
	shares, err := e.repo.GetAllTrashed()
	if err != nil {
		return err
	}

	var trashedIDs []string
	threshold := timestamp.Now().Time().Add(-24 * time.Hour)

	for _, s := range shares {
		if s.TrashedAt.Time().Before(threshold) {
			trashedIDs = append(trashedIDs, s.ID)
		}
	}

	if len(trashedIDs) == 0 {
		return nil
	}

	return e.repo.Delete(trashedIDs...)
}

func (e *Cleaner) deleteOrphaned() error {
	repoKeys, err := e.repo.GetKeys()
	if err != nil {
		return err
	}

	storageKeys, err := e.storage.GetKeys()
	if err != nil {
		return err
	}

	for key := range storageKeys {
		if _, ok := repoKeys[key]; ok {
			continue
		}

		shareID, fileID, isFile := strings.Cut(key, "/")
		if isFile {
			_ = e.storage.RemoveFile(shareID, fileID)
		} else {
			_ = e.storage.RemoveShareFolder(shareID)
		}
	}

	return nil
}
