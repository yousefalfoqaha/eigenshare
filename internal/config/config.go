package config

type Config struct {
	Port          int
	DataPath      string
	SecureCookies bool
	Secret        []byte
}

func Load() Config {
	return Config{
		Port:          8080,
		DataPath:      "data",
		SecureCookies: false,
		Secret:        []byte("haha12"),
	}
}
