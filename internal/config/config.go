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
}

func Load() Config {
	return Config{
		Port:          8080,
		DataPath:      "data",
		SecureCookies: false,
		Secret:        []byte("haha12"),
		TrustProxy:    true,
		RateLimit:     5,
		BurstLimit:    10,
	}
}
