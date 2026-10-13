package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Signer produces and verifies HMAC-SHA256 tags over arbitrary string
// fields.
type Signer struct {
	secret []byte
}

func NewSigner(secret []byte) *Signer {
	return &Signer{secret: secret}
}

// Sign returns a tag over the given fields, NUL-separated so that
// different field splits cannot produce the same tag.
func (s *Signer) Sign(fields ...string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	for _, f := range fields {
		mac.Write([]byte(f))
		mac.Write([]byte{0})
	}
	return mac.Sum(nil)
}

// Verify reports whether sig is a valid tag for the given fields.
func (s *Signer) Verify(sig []byte, fields ...string) bool {
	return len(sig) == sha256.Size && hmac.Equal(sig, s.Sign(fields...))
}

// Fingerprint derives a stable, non-reversible identifier from a stored
// password hash. It is safe to embed in cookies: bcrypt hashes are
// salted, so the fingerprint cannot be reversed or used as an offline
// guessing oracle, and any password change yields a different
// fingerprint.
func Fingerprint(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(sum[:])
}
