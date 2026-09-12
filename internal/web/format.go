package web

import (
	"fmt"
	"time"

	"eigenshare/internal/share"
	"eigenshare/internal/timestamp"
)

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func shareTotalSize(sh *share.Share) int64 {
	var total int64
	for _, f := range sh.Files {
		total += f.ExpectedSize
	}
	return total
}

func shareTotalFiles(sh *share.Share) int {
	return len(sh.Files)
}

func expiresIn(t *timestamp.Timestamp) string {
	if t == nil {
		return "never"
	}

	d := time.Until(t.Time())
	if d <= 0 {
		return "expired"
	}

	hours := int(d.Hours())
	if hours >= 24 {
		return fmt.Sprintf("in %d days", hours/24)
	}
	if hours > 0 {
		return fmt.Sprintf("in %d hours", hours)
	}

	minutes := int(d.Minutes())
	if minutes > 0 {
		return fmt.Sprintf("in %d mins", minutes)
	}

	return "in < 1 min"
}

func timeAgo(t *timestamp.Timestamp) string {
	if t == nil {
		return "never"
	}

	d := time.Since(t.Time())
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d mins ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}

	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}
