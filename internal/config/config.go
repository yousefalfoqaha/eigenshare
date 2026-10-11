package config

// Config holds application-wide settings. Values are hardcoded for now;
// this is the seam where env-var loading plugs in later.
type Config struct {
	Port          int
	DataPath      string
	SecureCookies bool
}

func Load() Config {
	return Config{
		Port:          8080,
		DataPath:      "data",
		SecureCookies: false,
	}
}
