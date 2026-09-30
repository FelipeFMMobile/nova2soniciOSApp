package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"stsmodel.local/agentwire"
	"time"
)

type Config struct {
	Environment, GatewayAddress, DevelopmentToken, Provider, DefaultAgentID string
	AgentCatalog                                                            []agentwire.Endpoint
	Principal                                                               agentwire.Principal
	ReadTimeout, WriteTimeout, IdleTimeout                                  time.Duration
	MaxEventBytes                                                           int64
	MaxSessions                                                             int
}

func Load() (Config, error) {
	if mode := os.Getenv("STS_AGENT_MODE"); mode != "" && mode != "remote" {
		return Config{}, fmt.Errorf("inprocess was removed; roll back the complete v0.10.x release")
	}
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		if value != "" && (strings.HasPrefix(key, "STS_LITELLM_") || strings.HasPrefix(key, "STS_MCP_") || strings.HasPrefix(key, "STS_NOVA_") || key == "STS_AGENT_PROMPT_FILE" || key == "STS_INTEGRATIONS_DIR" || key == "STS_TURN_TIMEOUT") {
			return Config{}, fmt.Errorf("%s belongs to the agent", key)
		}
	}
	for _, key := range []string{"STS_READ_TIMEOUT", "STS_WRITE_TIMEOUT", "STS_IDLE_TIMEOUT"} {
		if value := os.Getenv(key); value != "" {
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				return Config{}, fmt.Errorf("%s must be a positive duration", key)
			}
		}
	}
	for _, key := range []string{"STS_MAX_EVENT_BYTES", "STS_MAX_SESSIONS"} {
		if value := os.Getenv(key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return Config{}, fmt.Errorf("%s must be positive", key)
			}
		}
	}
	environment := env("STS_ENV", "development")
	principalID := os.Getenv("STS_PRINCIPAL_ID")
	if principalID == "" {
		if environment != "development" {
			return Config{}, fmt.Errorf("STS_PRINCIPAL_ID is required outside development")
		}
		principalID = "local-user"
	}
	cfg := Config{Environment: environment, GatewayAddress: env("STS_GATEWAY_ADDRESS", "127.0.0.1:8080"), DevelopmentToken: os.Getenv("STS_DEVELOPMENT_TOKEN"), Provider: env("STS_PROVIDER", "fake"), DefaultAgentID: env("STS_DEFAULT_AGENT_ID", "personal-assistant"), Principal: agentwire.Principal{Subject: principalID}, ReadTimeout: duration("STS_READ_TIMEOUT", 15*time.Second), WriteTimeout: duration("STS_WRITE_TIMEOUT", 15*time.Second), IdleTimeout: duration("STS_IDLE_TIMEOUT", 90*time.Second), MaxEventBytes: int64(integer("STS_MAX_EVENT_BYTES", 512*1024)), MaxSessions: integer("STS_MAX_SESSIONS", 16)}
	if err := agentwire.ValidatePrincipal(cfg.Principal); err != nil {
		return Config{}, err
	}
	catalog, err := loadAgentCatalog(os.Getenv("STS_AGENT_CATALOG"))
	if err != nil {
		return Config{}, err
	}
	cfg.AgentCatalog = catalog
	known := false
	for _, e := range catalog {
		known = known || e.AgentID == cfg.DefaultAgentID
	}
	if !known {
		return Config{}, fmt.Errorf("default agent absent from catalog")
	}
	host, _, err := net.SplitHostPort(cfg.GatewayAddress)
	if err != nil {
		return Config{}, fmt.Errorf("gateway address must be host:port")
	}
	ip := net.ParseIP(host)
	if cfg.DevelopmentToken == "" && (cfg.Environment != "development" || (host != "localhost" && (ip == nil || !ip.IsLoopback()))) {
		return Config{}, fmt.Errorf("development token required outside local development")
	}
	if cfg.Provider != "fake" && cfg.Provider != "nova" {
		return Config{}, fmt.Errorf("unsupported provider")
	}
	return cfg, nil
}

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func loadAgentCatalog(raw string) ([]agentwire.Endpoint, error) {
	var entries []struct {
		ID       string `json:"id"`
		Endpoint string `json:"endpoint"`
		Protocol string `json:"protocol"`
		TokenEnv string `json:"token_env"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("STS_AGENT_CATALOG must be a nonempty strict JSON array")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("STS_AGENT_CATALOG contains trailing JSON")
	}
	endpoints := make([]agentwire.Endpoint, 0, len(entries))
	for _, entry := range entries {
		if entry.Protocol != agentwire.Protocol || !environmentName.MatchString(entry.TokenEnv) || !strings.HasPrefix(entry.TokenEnv, "STS_AGENT_TOKEN") {
			return nil, fmt.Errorf("agent catalog protocol or token reference is invalid")
		}
		token, ok := os.LookupEnv(entry.TokenEnv)
		if !ok || token == "" {
			return nil, fmt.Errorf("agent catalog token reference is missing")
		}
		endpoints = append(endpoints, agentwire.Endpoint{AgentID: entry.ID, URL: entry.Endpoint, Token: token})
	}
	if _, err := agentwire.NewRemoteFactory(endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
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
