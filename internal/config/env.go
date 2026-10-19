package config

import (
	"fmt"
	"os"
	"strconv"
)

func requireEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		panic(fmt.Sprintf("missing required environment variable: %s", key))
	}
	return val
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func parseInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		panic(fmt.Sprintf("invalid integer value %q: %v", s, err))
	}
	return v
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid float value %q: %v", s, err))
	}
	return v
}

func parseBool(s string) bool {
	v, err := strconv.ParseBool(s)
	if err != nil {
		panic(fmt.Sprintf("invalid boolean value %q: %v", s, err))
	}
	return v
}
