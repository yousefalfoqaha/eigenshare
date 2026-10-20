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
)

const cookieFlash = "flash"

type cookies struct {
	secure bool
	secret []byte
}

func newCookies(secure bool, secret []byte) *cookies {
	return &cookies{secure: secure, secret: secret}
}

func (c *cookies) SetFlash(w http.ResponseWriter, kind, message string) {
	c.write(w, cookieFlash, kind+":"+message, "/", 5*time.Minute)
}

func (c *cookies) GetFlash(r *http.Request, w http.ResponseWriter) (kind, message string, ok bool) {
	raw, ok := c.read(r, cookieFlash)
	if !ok {
		return "", "", false
	}
	c.write(w, cookieFlash, "", "/", -1)
	return strings.Cut(raw, ":")
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
