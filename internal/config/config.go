package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"stsmodel.local/poc/internal/mcp"
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
	MCPServers        []mcp.ServerConfig
	MCPEvidencePath   string
	MCPCommand        string
	MCPArgs           []string
	MCPAllowedTools   []string
	MCPTimeout        time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxEventBytes     int64
	MaxSessions       int
	TurnTimeout       time.Duration
}

func Load() (Config, error) {
	for _, key := range []string{"STS_READ_TIMEOUT", "STS_WRITE_TIMEOUT", "STS_IDLE_TIMEOUT", "STS_TURN_TIMEOUT", "STS_NOVA_MAX_SESSION_AGE", "STS_MCP_TIMEOUT"} {
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
		MCPEvidencePath:   os.Getenv("STS_MCP_EVIDENCE_PATH"),
		Environment:       env("STS_ENV", "development"),
		GatewayAddress:    env("STS_GATEWAY_ADDRESS", "127.0.0.1:8080"),
		DevelopmentToken:  os.Getenv("STS_DEVELOPMENT_TOKEN"),
		Provider:          env("STS_PROVIDER", "fake"),
		NovaBridgeURL:     env("STS_NOVA_BRIDGE_URL", "ws://127.0.0.1:8091"),
		NovaMaxSessionAge: duration("STS_NOVA_MAX_SESSION_AGE", 7*time.Minute+30*time.Second),
		DatabasePath:      env("STS_DATABASE_PATH", "./data/sts.sqlite"),
		MCPCommand:        os.Getenv("STS_MCP_COMMAND"),
		MCPTimeout:        duration("STS_MCP_TIMEOUT", 10*time.Second),
		MCPAllowedTools:   strings.Split(env("STS_MCP_ALLOWED_TOOLS", "notes.create,notes.list,notes.delete"), ","),
		ReadTimeout:       duration("STS_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      duration("STS_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       duration("STS_IDLE_TIMEOUT", 90*time.Second),
		MaxEventBytes:     int64(integer("STS_MAX_EVENT_BYTES", 512*1024)),
		MaxSessions:       integer("STS_MAX_SESSIONS", 16),
		TurnTimeout:       duration("STS_TURN_TIMEOUT", 30*time.Second),
	}

	if raw := os.Getenv("STS_MCP_SERVERS"); raw != "" {
		if cfg.MCPCommand != "" {
			return Config{}, fmt.Errorf("use STS_MCP_SERVERS or STS_MCP_COMMAND, not both")
		}
		if err := json.Unmarshal([]byte(raw), &cfg.MCPServers); err != nil || len(cfg.MCPServers) == 0 {
			return Config{}, fmt.Errorf("STS_MCP_SERVERS must be a nonempty JSON server array")
		}
		if err := mcp.ValidateServers(cfg.MCPServers); err != nil {
			return Config{}, err
		}
	}
	if raw := os.Getenv("STS_MCP_ARGS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.MCPArgs); err != nil {
			return Config{}, fmt.Errorf("STS_MCP_ARGS must be a JSON string array")
		}
	}
	if cfg.MCPCommand != "" && !filepath.IsAbs(cfg.MCPCommand) {
		return Config{}, fmt.Errorf("STS_MCP_COMMAND must be an absolute executable path")
	}
	for i, name := range cfg.MCPAllowedTools {
		cfg.MCPAllowedTools[i] = strings.TrimSpace(name)
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
