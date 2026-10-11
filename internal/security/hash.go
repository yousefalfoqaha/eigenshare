package security

import (
	"crypto/rand"

	"golang.org/x/crypto/bcrypt"
)

const passwordHashCost = 12

func HashPassword(plain string) (string, error) {
	if len(plain) > 72 {
		return "", ErrPasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func VerifyPassword(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
