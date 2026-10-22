package config

import "golang.org/x/time/rate"

type Config struct {
	Port          int
	DataPath      string
	SecureCookies bool
	Secret        []byte
	TrustProxy    bool
	RateLimit     rate.Limit
	BurstLimit    int
	APIRateLimit  rate.Limit
	APIBurstLimit int
	ChunkSizeMB   int
}

func Load() Config {
	dataPath := getEnv("DATA_PATH", "data")
	return Config{
		Secret:        resolveSecret(dataPath),
		SecureCookies: parseBool(requireEnv("SECURE_COOKIES")),
		Port:          parseInt(getEnv("PORT", "8080")),
		DataPath:      dataPath,
		TrustProxy:    parseBool(getEnv("TRUST_PROXY", "false")),
		RateLimit:     rate.Limit(parseFloat(getEnv("RATE_LIMIT", "5"))),
		BurstLimit:    parseInt(getEnv("BURST_LIMIT", "10")),
		APIRateLimit:  rate.Limit(parseFloat(getEnv("API_RATE_LIMIT", "10"))),
		APIBurstLimit: parseInt(getEnv("API_BURST_LIMIT", "30")),
		ChunkSizeMB:   parseInt(getEnv("CHUNK_SIZE_MB", "5")),
	}
}
