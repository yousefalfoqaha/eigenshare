package share

import (
	"context"
	"time"
)

type ExpiryJob struct {
	interval     time.Duration
	shareService *ShareService
}

func NewExpiryJob(
	ctx context.Context,
	shareService *ShareService,
	interval time.Duration,
) *ExpiryJob {
	j := &ExpiryJob{interval, shareService}
	go j.run(ctx)

	return j
}

func (e *ExpiryJob) run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.cleanUpExpiredShares()
		}
	}
}

func (e *ExpiryJob) cleanUpExpiredShares() error {
	ids, err := e.shareService.GetExpiredShares()
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	return e.shareService.Delete(ids...)
}
