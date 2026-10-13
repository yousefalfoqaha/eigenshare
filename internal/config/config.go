package config

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
