package web

import (
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

// cookieProfile describes one kind of cookie the app writes. Every cookie
// the app uses is declared once in newCookies under a short key; call sites
// reference cookies by key only.
type cookieProfile struct {
	name   string
	maxAge time.Duration // converted to seconds only when the http.Cookie is built
	secret []byte        // only the signed unlock cookie uses this
}

// cookies writes and reads the app's cookies. The secure flag is shared by
// every cookie and fixed at construction.
type cookies struct {
	secure   bool
	profiles map[string]cookieProfile
}

func newCookies(secure bool, unlockSecret []byte) *cookies {
	return &cookies{
		secure: secure,
		profiles: map[string]cookieProfile{
			"session": {name: sessionCookieName, maxAge: session.TTL},
			"flash":   {name: flashCookieName}, // zero maxAge = session cookie
			"unlock":  {name: unlockCookieName, maxAge: share.UnlockTTL, secret: unlockSecret},
		},
	}
}

// lookup returns the profile for key, panicking on unknown kinds. Unknown
// keys are programmer errors, so failing fast beats a silent no-op.
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
	// TODO: your implementation — p := c.lookup(key); convert p.maxAge to
	// seconds (int(p.maxAge/time.Second)) where the http.Cookie is built.
}

// get reads the cookie of the given kind. The second return value is false
// when the cookie is missing or empty.
func (c *cookies) get(r *http.Request, key string) (string, bool) {
	// TODO: your implementation — p := c.lookup(key).
	return "", false
}

// clear deletes the cookie of the given kind by setting MaxAge to -1.
func (c *cookies) clear(w http.ResponseWriter, key string) {
	// TODO: your implementation — p := c.lookup(key).
}

// setSigned HMAC-signs msg (SHA-256 with the profile's secret), appends it
// to the list of unlocked items stored in the kind's cookie (keeping at most
// the last 30 entries), and writes the cookie with the profile's MaxAge.
//
// The stored payload has the form "{unixExpiry}|{csv list}" followed by a
// hex-encoded HMAC tag: "{payload}.{tag}". Messages that fail validIDPattern
// are silently dropped.
func (c *cookies) setSigned(w http.ResponseWriter, r *http.Request, key, msg string) {
	// TODO: your implementation — p := c.lookup(key); use p.secret and
	// p.maxAge.
}

// checkSigned reports whether msg is present in the kind's signed list of
// unlocked items. It returns false on any tamper, expiry or parse error.
func (c *cookies) checkSigned(r *http.Request, key, msg string) bool {
	// TODO: your implementation — p := c.lookup(key); use p.secret.
	return false
}
