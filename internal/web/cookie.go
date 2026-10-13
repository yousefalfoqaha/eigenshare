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
	signer *security.Signer
}

func newCookies(secure bool, signer *security.Signer) *cookies {
	return &cookies{secure: secure, signer: signer}
}

// SetSession issues a session cookie bound to the user's current
// password: the fingerprint embedded in the cookie invalidates it as
// soon as the password changes.
func (c *cookies) SetSession(w http.ResponseWriter, u *user.User) {
	c.write(w, CookieSession, fmt.Sprintf("%s:%s", u.ID, security.Fingerprint(u.PasswordHash)), "/", sessionTTL)
}

// GetSession returns the user id carried by a valid session cookie.
func (c *cookies) GetSession(r *http.Request) (id string, ok bool) {
	val, ok := c.read(r, CookieSession)
	if !ok {
		return "", false
	}
	id, _, ok = strings.Cut(val, ":")
	return id, ok
}

// SessionValid reports whether r's session cookie belongs to u: the
// fingerprint embedded in the cookie must match the fingerprint of u's
// current password hash, so rotating the password invalidates issued
// sessions.
func (c *cookies) SessionValid(r *http.Request, u *user.User) bool {
	val, ok := c.read(r, CookieSession)
	if !ok {
		return false
	}
	_, fp, ok := strings.Cut(val, ":")
	return ok && fp == security.Fingerprint(u.PasswordHash)
}

func (c *cookies) ClearSession(w http.ResponseWriter) {
	c.write(w, CookieSession, "", "/", -1)
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

// SetUnlock issues an unlock cookie bound to the share's current
// password: changing the share's password invalidates every cookie
// issued under the old one.
func (c *cookies) SetUnlock(w http.ResponseWriter, sh *share.Share) {
	c.write(w, CookieUnlockPrefix+sh.ID, fmt.Sprintf("%s:%s", sh.ID, security.Fingerprint(sh.PasswordHash)), "/s/", share.UnlockTTL)
}

// HasUnlock reports whether the share is unlocked with a cookie issued
// against its current password. Invalid or stale cookies are cleared.
func (c *cookies) HasUnlock(r *http.Request, w http.ResponseWriter, sh *share.Share) bool {
	cookieName := CookieUnlockPrefix + sh.ID

	if _, err := r.Cookie(cookieName); err != nil {
		return false
	}

	val, ok := c.read(r, cookieName)
	if !ok {
		return false
	}

	id, fp, ok := strings.Cut(val, ":")
	if !ok || id != sh.ID || fp != security.Fingerprint(sh.PasswordHash) {
		c.write(w, cookieName, "", "/s/", -1)
		return false
	}

	return true
}

func (c *cookies) ClearUnlock(w http.ResponseWriter, id string) {
	cookieName := CookieUnlockPrefix + id
	c.write(w, cookieName, "", "/s/", -1)
}

func (c *cookies) write(w http.ResponseWriter, name, value, path string, ttl time.Duration) {
	expires := time.Now().Add(ttl).Unix()
	expStr := strconv.FormatInt(expires, 10)

	payload := fmt.Sprintf("%s:%s", value, expStr)

	signedValue := append(c.signer.Sign(name, value, expStr), payload...)
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

	if !c.signer.Verify(signature, name, value, expStr) {
		return "", false
	}

	expires, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}

	return value, true
}
