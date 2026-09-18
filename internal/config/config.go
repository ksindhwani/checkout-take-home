// Package config loads runtime configuration from environment variables,
// each with a default that works out of the box for local development.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr        string
	BankBaseURL string
	BankTimeout time.Duration
	LogLevel    string
}

func Load() Config {
	if err := godotenv.Load(); err != nil {
		fmt.Println("no .env file found, using real environment variables")
	}

	return Config{
		Addr:        ":" + getEnv("PORT", "8090"),
		BankBaseURL: getEnv("BANK_SIMULATOR_BASE_URL", "http://localhost:8080"),
		BankTimeout: getEnvDuration("BANK_CALL_TIMEOUT", 5*time.Second),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return fallback
}
