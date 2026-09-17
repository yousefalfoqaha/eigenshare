package views

import (
	"net/http"

	"eigenshare/internal/auth"
	"eigenshare/internal/share"
)

type EditShareContent struct {
	shareService *share.ShareService
	Share        *share.Share
	Progress     map[string]int64
}

func NewEditShareContent(shareService *share.ShareService) *EditShareContent {
	return &EditShareContent{shareService: shareService}
}

func (c *EditShareContent) Load(r *http.Request) error {
	actor := auth.UserFromContext(r.Context())
	sh, err := c.shareService.GetShare(r.PathValue("id"), actor)
	if err != nil {
		return err
	}
	c.Share = sh
	c.Progress, _ = c.shareService.GetShareProgress(sh.ID, actor)
	return nil
}

func (c *EditShareContent) Title() string {
	if c.Share == nil {
		return "edit share"
	}
	return c.Share.Handle
}
