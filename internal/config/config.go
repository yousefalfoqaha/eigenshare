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
}

func Load() Config {
	return Config{
		Secret:        []byte(requireEnv("SECRET")),
		SecureCookies: parseBool(requireEnv("SECURE_COOKIES")),
		Port:          parseInt(getEnv("PORT", "8080")),
		DataPath:      getEnv("DATA_PATH", "data"),
		TrustProxy:    parseBool(getEnv("TRUST_PROXY", "false")),
		RateLimit:     rate.Limit(parseFloat(getEnv("RATE_LIMIT", "5"))),
		BurstLimit:    parseInt(getEnv("BURST_LIMIT", "10")),
		APIRateLimit:  rate.Limit(parseFloat(getEnv("API_RATE_LIMIT", "10"))),
		APIBurstLimit: parseInt(getEnv("API_BURST_LIMIT", "30")),
	}
}
