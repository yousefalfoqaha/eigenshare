package share

import (
	"context"
	"log"
	"time"
)

type CleanupJob struct {
	interval     time.Duration
	shareService *ShareService
}

func NewCleanupJob(
	ctx context.Context,
	shareService *ShareService,
	interval time.Duration,
) *CleanupJob {
	j := &CleanupJob{interval, shareService}
	go j.run(ctx)

	return j
}

func (e *CleanupJob) run(ctx context.Context) {
	e.runMaintenance()

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runMaintenance()
		}
	}
}

func (e *CleanupJob) runMaintenance() {
	if err := e.cleanUpExpiredShares(); err != nil {
		log.Printf("failed to clean up expired shares: %v", err)
	}
	if err := e.cleanUpOrphanedShareFolders(); err != nil {
		log.Printf("failed to clean up orphaned share folders: %v", err)
	}
}

func (e *CleanupJob) cleanUpExpiredShares() error {
	ids, err := e.shareService.GetExpiredShares()
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	return e.shareService.Delete(ids...)
}

func (e *CleanupJob) cleanUpOrphanedShareFolders() error {
	summaries, err := e.shareService.shareRepository.GetAll()
	if err != nil {
		return err
	}

	validIDs := make([]string, len(summaries))
	for i, summary := range summaries {
		validIDs[i] = summary.ID
	}

	shareIDs, err := e.shareService.fileStorage.GetShareIDs()
	if err != nil {
		return err
	}

	valid := make(map[string]struct{}, len(validIDs))
	for _, id := range validIDs {
		valid[id] = struct{}{}
	}

	var orphanedIDs []string
	for _, id := range shareIDs {
		if _, exists := valid[id]; !exists {
			orphanedIDs = append(orphanedIDs, id)
		}
	}

	if len(orphanedIDs) == 0 {
		return nil
	}

	return e.shareService.fileStorage.RemoveShareFolder(orphanedIDs...)
}
