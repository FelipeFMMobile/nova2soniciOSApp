package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment      string
	GatewayAddress   string
	DevelopmentToken string
	Provider         string
	NovaBridgeURL    string
	DatabasePath     string
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	IdleTimeout      time.Duration
	MaxEventBytes    int64
}

func Load() (Config, error) {
	cfg := Config{
		Environment:      env("STS_ENV", "development"),
		GatewayAddress:   env("STS_GATEWAY_ADDRESS", "127.0.0.1:8080"),
		DevelopmentToken: os.Getenv("STS_DEVELOPMENT_TOKEN"),
		Provider:         env("STS_PROVIDER", "fake"),
		NovaBridgeURL:    env("STS_NOVA_BRIDGE_URL", "ws://127.0.0.1:8091"),
		DatabasePath:     env("STS_DATABASE_PATH", "./data/sts.sqlite"),
		ReadTimeout:      duration("STS_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:     duration("STS_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:      duration("STS_IDLE_TIMEOUT", 90*time.Second),
		MaxEventBytes:    int64(integer("STS_MAX_EVENT_BYTES", 512*1024)),
	}

	if cfg.Environment != "development" && cfg.DevelopmentToken == "" {
		return Config{}, fmt.Errorf("STS_DEVELOPMENT_TOKEN is required outside development")
	}
	if cfg.Provider != "fake" && cfg.Provider != "nova" {
		return Config{}, fmt.Errorf("unsupported STS_PROVIDER %q", cfg.Provider)
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func integer(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
