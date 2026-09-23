package app

import (
	"fmt"
	"strings"
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

func formatCommaSeparated(items []string) string {
	return strings.Join(items, ", ")
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
		return "now (cleaning up)"
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

// dict builds a map from key/value pairs so a template component can be
// invoked with several named arguments: {{template "x" (dict "a" 1 "b" 2)}}
func dict(kv ...any) map[string]any {
	if len(kv)%2 != 0 {
		panic("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}
