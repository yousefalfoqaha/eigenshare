package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

const (
	pbkdf2Iterations = 600_000
	saltLen          = 32
	keyLen           = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	dk := pbkdf2Key([]byte(password), salt, pbkdf2Iterations, keyLen, sha256.New)
	return encodeHash(pbkdf2Iterations, salt, dk), nil
}

func VerifyPassword(password, stored string) (bool, error) {
	iter, salt, want, err := decodeHash(stored)
	if err != nil {
		return false, err
	}

	got := pbkdf2Key([]byte(password), salt, iter, keyLen, sha256.New)
	return subtle.ConstantTimeCompare(want, got) == 1, nil
}

func encodeHash(iter int, salt, dk []byte) string {
	return "pbkdf2-sha256:" + strconv.Itoa(iter) + ":" +
		base64.StdEncoding.EncodeToString(salt) + ":" +
		base64.StdEncoding.EncodeToString(dk)
}

func decodeHash(stored string) (iter int, salt, dk []byte, err error) {
	parts := strings.Split(stored, ":")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return 0, nil, nil, errors.New("invalid password hash format")
	}

	iter, err = strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return 0, nil, nil, errors.New("invalid iteration count")
	}

	salt, err = base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("decode salt: %w", err)
	}

	dk, err = base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("decode hash: %w", err)
	}

	return iter, salt, dk, nil
}

func pbkdf2Key(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	dk := make([]byte, 0, numBlocks*hashLen)
	U := make([]byte, hashLen)
	var buf [4]byte

	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:])
		dk = prf.Sum(dk)

		blockOff := (block - 1) * hashLen
		copy(U, dk[blockOff:])

		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(U)
			U = prf.Sum(U[:0])
			for i := range U {
				dk[blockOff+i] ^= U[i]
			}
		}
	}

	return dk[:keyLen]
}
