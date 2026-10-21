package security

import (
	"crypto/rand"
	"encoding/hex"
)

func GenerateRandom(length int) ([]byte, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return []byte(hex.EncodeToString(bytes)), nil
}
