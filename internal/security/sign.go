package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func Sign(secret []byte, fields ...string) []byte {
	mac := hmac.New(sha256.New, secret)
	for _, f := range fields {
		mac.Write([]byte(f))
		mac.Write([]byte{0})
	}
	return mac.Sum(nil)
}

func VerifySignature(secret []byte, sig []byte, fields ...string) bool {
	return len(sig) == sha256.Size && hmac.Equal(sig, Sign(secret, fields...))
}

func Fingerprint(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(sum[:])
}
