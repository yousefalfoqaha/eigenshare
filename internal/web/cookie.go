package web

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eigenshare/internal/security"
	"eigenshare/internal/share"
	"eigenshare/internal/user"
)

const (
	CookieSession      = "session_id"
	CookieFlash        = "flash"
	CookieUnlockPrefix = "share_unlock_"

	sessionTTL = 7 * 24 * time.Hour
)

type cookies struct {
	secure bool
	secret []byte
}

func newCookies(secure bool, secret []byte) *cookies {
	return &cookies{secure: secure, secret: secret}
}

func (c *cookies) SetFlash(w http.ResponseWriter, kind, message string) {
	c.write(w, CookieFlash, kind+":"+message, "/", 5*time.Minute)
}

func (c *cookies) GetFlash(r *http.Request, w http.ResponseWriter) (kind, message string, ok bool) {
	raw, ok := c.read(r, CookieFlash)
	if !ok {
		return "", "", false
	}
	c.write(w, CookieFlash, "", "/", -1)
	return strings.Cut(raw, ":")
}

func (c *cookies) SetSession(w http.ResponseWriter, u *user.User) {
	c.write(w, CookieSession, fmt.Sprintf("%s:%s", u.ID, security.Fingerprint(u.PasswordHash)), "/", sessionTTL)
}

func (c *cookies) GetSession(r *http.Request) (id, fp string, ok bool) {
	val, ok := c.read(r, CookieSession)
	if !ok {
		return "", "", false
	}
	id, fp, ok = strings.Cut(val, ":")
	return id, fp, ok
}

func (c *cookies) ClearSession(w http.ResponseWriter) {
	c.write(w, CookieSession, "", "/", -1)
}

func (c *cookies) SetUnlock(w http.ResponseWriter, sh *share.Share) {
	c.write(w, CookieUnlockPrefix+sh.ID, fmt.Sprintf("%s:%s", sh.ID, security.Fingerprint(sh.PasswordHash)), "/", share.UnlockTTL)
}

func (c *cookies) GetUnlock(r *http.Request, shareID string) (id, fp string, ok bool) {
	val, ok := c.read(r, CookieUnlockPrefix+shareID)
	if !ok {
		return "", "", false
	}
	id, fp, ok = strings.Cut(val, ":")
	return id, fp, ok
}

func (c *cookies) ClearUnlock(w http.ResponseWriter, id string) {
	c.write(w, CookieUnlockPrefix+id, "", "/", -1)
}

// uploadFileCookiePrefix names the cookies holding files registered through
// an upload request. Each cookie carries one file id.
const uploadFileCookiePrefix = "upload_file_"

// uploadFileCookieTTL is how long an upload file cookie lives when the
// request has no expiry. Registration and page loads refresh the cookies
// they still care about.
const uploadFileCookieTTL = 365 * 24 * time.Hour

func uploadFileCookieName(requestID, fileID string) string {
	return uploadFileCookiePrefix + requestID + "_" + fileID
}

// SetUploadFile remembers a file registered through the upload request.
func (c *cookies) SetUploadFile(w http.ResponseWriter, req *share.UploadRequest, fileID string) {
	ttl := uploadFileCookieTTL
	if req.ExpiresAt != nil {
		if remaining := time.Until(req.ExpiresAt.Time()); remaining > 0 {
			ttl = remaining
		}
	}

	c.write(w, uploadFileCookieName(req.ID, fileID), "1", "/", ttl)
}

// ClearUploadFile forgets a file previously registered through the upload
// request.
func (c *cookies) ClearUploadFile(w http.ResponseWriter, req *share.UploadRequest, fileID string) {
	c.write(w, uploadFileCookieName(req.ID, fileID), "", "/", -1)
}

// GetUploadFileIDs returns the files registered through the upload request
// by this client, or nil when none are remembered. Only cookies with a
// valid signature are considered.
func (c *cookies) GetUploadFileIDs(r *http.Request, requestID string) []string {
	prefix := uploadFileCookiePrefix + requestID + "_"

	var fileIDs []string
	for _, cookie := range r.Cookies() {
		if !strings.HasPrefix(cookie.Name, prefix) {
			continue
		}
		fileID := strings.TrimPrefix(cookie.Name, prefix)
		if fileID == "" {
			continue
		}
		if v, ok := c.read(r, cookie.Name); !ok || v != "1" {
			continue
		}
		fileIDs = append(fileIDs, fileID)
	}

	return fileIDs
}

func (c *cookies) write(w http.ResponseWriter, name, value, path string, ttl time.Duration) {
	expires := time.Now().Add(ttl).Unix()
	expStr := strconv.FormatInt(expires, 10)

	payload := fmt.Sprintf("%s:%s", value, expStr)

	signedValue := append(security.Sign(c.secret, name, value, expStr), payload...)
	encoded := base64.URLEncoding.EncodeToString(signedValue)

	maxAge := int(ttl / time.Second)
	if ttl < 0 {
		maxAge = -1
	}

	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    encoded,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (c *cookies) read(r *http.Request, name string) (string, bool) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", false
	}

	decoded, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil || len(decoded) <= sha256.Size {
		return "", false
	}

	signature := decoded[:sha256.Size]
	payload := string(decoded[sha256.Size:])

	lastColon := strings.LastIndex(payload, ":")
	if lastColon == -1 {
		return "", false
	}

	value := payload[:lastColon]
	expStr := payload[lastColon+1:]

	if !security.VerifySignature(c.secret, signature, name, value, expStr) {
		return "", false
	}

	expires, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}

	return value, true
}
