package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Environment       string
	GatewayAddress    string
	DevelopmentToken  string
	Provider          string
	NovaBridgeURL     string
	NovaMaxSessionAge time.Duration
	DatabasePath      string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxEventBytes     int64
	MaxSessions       int
	TurnTimeout       time.Duration
}

func Load() (Config, error) {
	for _, key := range []string{"STS_READ_TIMEOUT", "STS_WRITE_TIMEOUT", "STS_IDLE_TIMEOUT", "STS_TURN_TIMEOUT", "STS_NOVA_MAX_SESSION_AGE"} {
		if value := os.Getenv(key); value != "" {
			parsed, err := time.ParseDuration(value)
			if err != nil || parsed <= 0 {
				return Config{}, fmt.Errorf("%s must be a positive duration", key)
			}
		}
	}
	for _, key := range []string{"STS_MAX_EVENT_BYTES", "STS_MAX_SESSIONS"} {
		if value := os.Getenv(key); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return Config{}, fmt.Errorf("%s must be a positive integer", key)
			}
		}
	}
	cfg := Config{
		Environment:       env("STS_ENV", "development"),
		GatewayAddress:    env("STS_GATEWAY_ADDRESS", "127.0.0.1:8080"),
		DevelopmentToken:  os.Getenv("STS_DEVELOPMENT_TOKEN"),
		Provider:          env("STS_PROVIDER", "fake"),
		NovaBridgeURL:     env("STS_NOVA_BRIDGE_URL", "ws://127.0.0.1:8091"),
		NovaMaxSessionAge: duration("STS_NOVA_MAX_SESSION_AGE", 7*time.Minute+30*time.Second),
		DatabasePath:      env("STS_DATABASE_PATH", "./data/sts.sqlite"),
		ReadTimeout:       duration("STS_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      duration("STS_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       duration("STS_IDLE_TIMEOUT", 90*time.Second),
		MaxEventBytes:     int64(integer("STS_MAX_EVENT_BYTES", 512*1024)),
		MaxSessions:       integer("STS_MAX_SESSIONS", 16),
		TurnTimeout:       duration("STS_TURN_TIMEOUT", 30*time.Second),
	}

	if cfg.Environment != "development" && cfg.DevelopmentToken == "" {
		return Config{}, fmt.Errorf("STS_DEVELOPMENT_TOKEN is required outside development")
	}
	host, _, err := net.SplitHostPort(cfg.GatewayAddress)
	if err != nil {
		return Config{}, fmt.Errorf("STS_GATEWAY_ADDRESS must be host:port")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) && cfg.DevelopmentToken == "" {
		return Config{}, fmt.Errorf("STS_DEVELOPMENT_TOKEN is required when listening outside loopback")
	}
	if cfg.Provider != "fake" && cfg.Provider != "nova" {
		return Config{}, fmt.Errorf("unsupported STS_PROVIDER %q", cfg.Provider)
	}
	if cfg.NovaMaxSessionAge > 7*time.Minute+30*time.Second {
		return Config{}, fmt.Errorf("STS_NOVA_MAX_SESSION_AGE must not exceed 7m30s")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.TurnTimeout <= 0 {
		return Config{}, fmt.Errorf("timeouts must be positive")
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
