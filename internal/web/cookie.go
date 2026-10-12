package web

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"eigenshare/internal/session"
	"eigenshare/internal/share"
)

const (
	sessionCookieName = "session_id"
	unlockCookieName  = "share_unlock"
	flashCookieName   = "flash"
)

type cookieProfile struct {
	path   string
	maxAge time.Duration
}

type cookies struct {
	secure   bool
	profiles map[string]cookieProfile
	secret   []byte
}

type cookiePayload struct {
	value      string
	expiration int // in seconds
}

func newCookies(secure bool, secret []byte) *cookies {
	return &cookies{
		secure: secure,
		profiles: map[string]cookieProfile{
			"session": {maxAge: session.TTL},
			"flash":   {maxAge: 0},
			"unlock":  {path: "/s/", maxAge: share.UnlockTTL},
		},
		secret: secret,
	}
}

func (c *cookies) lookup(key string) cookieProfile {
	p, ok := c.profiles[key]
	if !ok {
		panic("web: unknown cookie kind: " + key)
	}
	return p
}

// set writes a cookie of the given kind with Path "/", HttpOnly,
// SameSite=Lax, MaxAge from the profile and Secure=c.secure.
func (c *cookies) set(w http.ResponseWriter, key, value string) {
	profile := c.lookup(key)

	payload, err := json.Marshal(
		cookiePayload{
			value:      value,
			expiration: int(profile.maxAge / time.Second),
		},
	)
	if err != nil {
		return
	}

	encodedValue := base64.URLEncoding.EncodeToString(payload)

	path := "/"
	if profile.path != "" {
		path = profile.path
	}

	cookie := http.Cookie{
		Name:     key,
		Value:    encodedValue,
		Path:     path,
		MaxAge:   int(profile.maxAge / time.Second),
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}

	if len(cookie.String()) > 4096 {
		return
	}

	http.SetCookie(w, &cookie)
}

// get reads the cookie of the given kind. The second return value is false
// when the cookie is missing or empty.
func (c *cookies) get(r *http.Request, key string) (string, bool) {
	cookie, err := r.Cookie(key)
	if err != nil {
		return "", false
	}

	value, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return "", false
	}

	return string(value), true
}

// clear deletes the cookie of the given kind by setting MaxAge to -1.
func (c *cookies) clear(w http.ResponseWriter, key string) {
	cookie := http.Cookie{
		Name:     key,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, &cookie)
}

// setSigned HMAC-signs msg (SHA-256 with the profile's secret), appends it
// to the list of unlocked items stored in the kind's cookie (keeping at most
// the last 30 entries), and writes the cookie with the profile's MaxAge.
//
// The stored payload has the form "{unixExpiry}|{csv list}" followed by a
// hex-encoded HMAC tag: "{payload}.{tag}". Messages that fail validIDPattern
// are silently dropped.
func (c *cookies) setSigned(w http.ResponseWriter, r *http.Request, key, msg string) {
	profile := c.lookup(key)

	payload, err := json.Marshal(
		cookiePayload{
			value:      msg,
			expiration: int(profile.maxAge / time.Second),
		},
	)
	if err != nil {
		return
	}

	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(key))
	mac.Write([]byte(msg))
	signature := mac.Sum(nil)

	signedValue := string(signature) + string(payload)
	c.write(w, key, signedValue)
}

func (c *cookies) write(w http.ResponseWriter, key string, value string) {
	profile := c.lookup(key)

	payload, err := json.Marshal(
		cookiePayload{
			value:      value,
			expiration: int(profile.maxAge / time.Second),
		},
	)
	if err != nil {
		return
	}

	encodedValue := base64.URLEncoding.EncodeToString(payload)

	path := "/"
	if profile.path != "" {
		path = profile.path
	}

	cookie := http.Cookie{
		Name:     key,
		Value:    encodedValue,
		Path:     path,
		MaxAge:   int(profile.maxAge / time.Second),
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}

	if len(cookie.String()) > 4096 {
		return
	}

	http.SetCookie(w, &cookie)
}

func (c *cookies) read(r *http.Request, key string) (string, bool) {
	cookie, err := r.Cookie(key)
	if err != nil {
		return "", false
	}

	signedValue, err := base64.URLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return "", false
	}

	return string(signedValue), true
}

func (c *cookies) getSigned(r *http.Request, key string) (string, bool) {
	signedValue, ok := c.read(r, key)
	if !ok {
		return "", false
	}

	if len(signedValue) < sha256.Size {
		return "", false
	}

	signature := signedValue[:sha256.Size]
	value := signedValue[sha256.Size:]

	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(key))
	mac.Write([]byte(value))
	expectedSignature := mac.Sum(nil)

	if !hmac.Equal([]byte(signature), expectedSignature) {
		return "", false
	}

	return value, true
}

// checkSigned reports whether msg is present in the kind's signed list of
// unlocked items. It returns false on any tamper, expiry or parse error.
func (c *cookies) checkSigned(r *http.Request, key, msg string) bool {
	// TODO: your implementation — p := c.lookup(key); use p.secret.
	return false
}
