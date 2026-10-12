package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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

func (c *cookies) SetSession(w http.ResponseWriter, u *user.User) {
	c.write(w, CookieSession, fmt.Sprintf("%s:%d", u.ID, u.Version), "/", sessionTTL)
}

func (c *cookies) GetSession(r *http.Request) (id, version string, ok bool) {
	val, ok := c.read(r, CookieSession)
	if !ok {
		return "", "", false
	}
	id, version, ok = strings.Cut(val, ":")
	return id, version, ok
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

func (c *cookies) SetUnlock(w http.ResponseWriter, id string) {
	c.write(w, CookieUnlockPrefix+id, id, "/s/", share.UnlockTTL)
}

func (c *cookies) HasUnlock(r *http.Request, id string) bool {
	val, ok := c.read(r, CookieUnlockPrefix+id)
	return ok && val == id
}

func (c *cookies) write(w http.ResponseWriter, name, value, path string, ttl time.Duration) {
	expires := time.Now().Add(ttl).Unix()
	expStr := strconv.FormatInt(expires, 10)

	payload := fmt.Sprintf("%s:%s", value, expStr)

	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(name))
	mac.Write([]byte(value))
	mac.Write([]byte(expStr))

	signedValue := append(mac.Sum(nil), payload...)
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

	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(name))
	mac.Write([]byte(value))
	mac.Write([]byte(expStr))

	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", false
	}

	expires, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}

	return value, true
}
