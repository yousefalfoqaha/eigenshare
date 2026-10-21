package config

import (
	"log"
	"os"
	"path/filepath"

	"eigenshare/internal/security"
)

const secretLength int = 32

func resolveSecret(dataPath string) []byte {
	if val := os.Getenv("SECRET"); val != "" {
		return []byte(val)
	}

	secretPath := filepath.Join(dataPath, ".secret")
	if data, err := os.ReadFile(secretPath); err == nil && len(data) > 0 {
		return data
	}

	generated, err := security.GenerateRandom(secretLength)
	if err != nil {
		log.Fatalf("failed to generate secret: %v", err)
	}

	if err := os.MkdirAll(dataPath, 0o700); err == nil {
		if err := os.WriteFile(secretPath, generated, 0o600); err != nil {
			log.Printf("warning: could not persist generated secret: %v", err)
		}
	}

	return generated
}
