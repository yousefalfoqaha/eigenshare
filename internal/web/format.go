package web

import (
	"fmt"
	"strings"
	"time"

	"eigenshare/internal/share"
	"eigenshare/internal/user"
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

func formatOwners(ownerIDs []string, users []*user.User) string {
	byID := make(map[string]string, len(users))
	for _, u := range users {
		byID[u.ID] = u.Username
	}
	names := make([]string, 0, len(ownerIDs))
	for _, id := range ownerIDs {
		if name, ok := byID[id]; ok {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func uploadedFiles(states map[string]share.FileUploadState, files []share.File) []share.File {
	uploaded := make([]share.File, 0, len(files))
	for _, f := range files {
		if f.Complete(states[f.ID].Written) {
			uploaded = append(uploaded, f)
		}
	}
	return uploaded
}

func uploadedSize(files []share.File) int64 {
	var total int64
	for _, f := range files {
		total += f.ExpectedSize
	}
	return total
}

func expiresIn(t *time.Time) string {
	if t == nil {
		return "never"
	}

	d := time.Until(*t)
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

func timeAgo(t *time.Time) string {
	if t == nil {
		return "never"
	}

	d := time.Since(*t)
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
