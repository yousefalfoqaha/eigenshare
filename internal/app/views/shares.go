package views

import "eigenshare/internal/share"

type SharesContent struct {
	Shares   []share.ShareSummary
	Progress map[string]int64
}
