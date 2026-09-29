package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"stsmodel.local/agentwire"
	agentconfig "stsmodel.local/personal-assistant-agent/config"
	"time"
)

type Config struct {
	Environment       string
	GatewayAddress    string
	DevelopmentToken  string
	Provider          string
	AgentMode         string
	DefaultAgentID    string
	AgentCatalog      []agentwire.Endpoint
	AgentPromptFile   string
	NovaMaxSessionAge time.Duration
	LiteLLMModel      string
	MCPServers        []agentconfig.ServerConfig
	MCPEvidencePath   string
	MCPCommand        string
	MCPArgs           []string
	MCPAllowedTools   []string
	MCPTimeout        time.Duration
	MCPBackend        string
	LiteLLMURL        string
	LiteLLMAPIKey     string
	MCPContextSecret  string
	LiteLLMServers    []agentconfig.GatewayServerConfig
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
		AgentMode:         env("STS_AGENT_MODE", "remote"),
		DefaultAgentID:    env("STS_DEFAULT_AGENT_ID", "personal-assistant"),
		AgentPromptFile:   os.Getenv("STS_AGENT_PROMPT_FILE"),
		NovaMaxSessionAge: duration("STS_NOVA_MAX_SESSION_AGE", 7*time.Minute+30*time.Second),
		LiteLLMModel:      env("STS_LITELLM_REALTIME_MODEL", "nova-sonic"),
		MCPCommand:        os.Getenv("STS_MCP_COMMAND"),
		MCPTimeout:        duration("STS_MCP_TIMEOUT", 10*time.Second),
		MCPBackend:        env("STS_MCP_BACKEND", "stdio"),
		LiteLLMURL:        os.Getenv("STS_LITELLM_URL"),
		LiteLLMAPIKey:     os.Getenv("STS_LITELLM_API_KEY"),
		MCPContextSecret:  os.Getenv("STS_MCP_CONTEXT_SECRET"),
		MCPAllowedTools:   strings.Split(env("STS_MCP_ALLOWED_TOOLS", "notes.create,notes.list,notes.delete"), ","),
		ReadTimeout:       duration("STS_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:      duration("STS_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:       duration("STS_IDLE_TIMEOUT", 90*time.Second),
		MaxEventBytes:     int64(integer("STS_MAX_EVENT_BYTES", 512*1024)),
		MaxSessions:       integer("STS_MAX_SESSIONS", 16),
		TurnTimeout:       duration("STS_TURN_TIMEOUT", 30*time.Second),
	}
	if cfg.AgentMode != "inprocess" && cfg.AgentMode != "remote" {
		return Config{}, fmt.Errorf("STS_AGENT_MODE must be inprocess or remote")
	}
	if cfg.AgentMode == "remote" {
		for _, key := range []string{"STS_AGENT_PROMPT_FILE", "STS_LITELLM_URL", "STS_LITELLM_API_KEY", "STS_LITELLM_REALTIME_MODEL", "STS_MCP_BACKEND", "STS_MCP_COMMAND", "STS_MCP_ARGS", "STS_MCP_SERVERS", "STS_LITELLM_MCP_SERVERS", "STS_MCP_CONTEXT_SECRET", "STS_MCP_EVIDENCE_PATH", "STS_MCP_ALLOWED_TOOLS", "STS_INTEGRATIONS_DIR"} {
			if os.Getenv(key) != "" {
				return Config{}, fmt.Errorf("%s belongs to the remote agent, not the gateway", key)
			}
		}
		catalog, err := loadAgentCatalog(os.Getenv("STS_AGENT_CATALOG"))
		if err != nil {
			return Config{}, err
		}
		cfg.AgentCatalog = catalog
		known := false
		for _, endpoint := range catalog {
			known = known || endpoint.AgentID == cfg.DefaultAgentID
		}
		if !known {
			return Config{}, fmt.Errorf("default agent is absent from trusted catalog")
		}
		cfg.AgentPromptFile, cfg.LiteLLMModel, cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.MCPContextSecret = "", "", "", "", ""
		cfg.MCPServers, cfg.LiteLLMServers, cfg.MCPArgs, cfg.MCPAllowedTools = nil, nil, nil, nil
		cfg.MCPCommand, cfg.MCPEvidencePath, cfg.MCPBackend = "", "", ""
	}
	if cfg.AgentMode == "inprocess" && cfg.MCPBackend != "stdio" && cfg.MCPBackend != "litellm" {
		return Config{}, fmt.Errorf("STS_MCP_BACKEND must be stdio or litellm")
	}
	if cfg.AgentMode == "inprocess" && cfg.MCPBackend == "litellm" {
		if cfg.MCPCommand != "" || os.Getenv("STS_MCP_SERVERS") != "" {
			return Config{}, fmt.Errorf("LiteLLM backend cannot use stdio MCP configuration")
		}
		if err := json.Unmarshal([]byte(os.Getenv("STS_LITELLM_MCP_SERVERS")), &cfg.LiteLLMServers); err != nil {
			return Config{}, fmt.Errorf("STS_LITELLM_MCP_SERVERS must be a JSON server array")
		}
		if cfg.LiteLLMURL == "" || cfg.LiteLLMAPIKey == "" {
			return Config{}, fmt.Errorf("LiteLLM URL and API key required")
		}
	}
	if cfg.AgentMode == "inprocess" && cfg.Provider == "nova" && (cfg.LiteLLMURL == "" || cfg.LiteLLMAPIKey == "" || strings.TrimSpace(cfg.LiteLLMModel) == "") {
		return Config{}, fmt.Errorf("Nova requires LiteLLM URL, API key and realtime model")
	}
	if cfg.AgentMode == "inprocess" && cfg.Provider == "nova" {
		if _, err := agentconfig.LoadPrompt(cfg.AgentPromptFile); err != nil {
			return Config{}, err
		}
	}
	if cfg.AgentMode == "inprocess" && cfg.LiteLLMURL != "" {
		parsed, err := url.Parse(cfg.LiteLLMURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return Config{}, fmt.Errorf("STS_LITELLM_URL must be an HTTP(S) base URL")
		}
	}

	if raw := os.Getenv("STS_MCP_SERVERS"); cfg.AgentMode == "inprocess" && raw != "" {
		if cfg.MCPCommand != "" {
			return Config{}, fmt.Errorf("use STS_MCP_SERVERS or STS_MCP_COMMAND, not both")
		}
		if err := json.Unmarshal([]byte(raw), &cfg.MCPServers); err != nil || len(cfg.MCPServers) == 0 {
			return Config{}, fmt.Errorf("STS_MCP_SERVERS must be a nonempty JSON server array")
		}
	}
	if raw := os.Getenv("STS_MCP_ARGS"); cfg.AgentMode == "inprocess" && raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.MCPArgs); err != nil {
			return Config{}, fmt.Errorf("STS_MCP_ARGS must be a JSON string array")
		}
	}
	if cfg.AgentMode == "inprocess" && cfg.MCPCommand != "" && !filepath.IsAbs(cfg.MCPCommand) {
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

func (c Config) Agent() agentconfig.Config {
	return agentconfig.Config{
		Provider: c.Provider, PromptFile: c.AgentPromptFile,
		NovaMaxSessionAge: c.NovaMaxSessionAge, LiteLLMModel: c.LiteLLMModel,
		MCPServers: c.MCPServers, MCPEvidencePath: c.MCPEvidencePath,
		MCPCommand: c.MCPCommand, MCPArgs: c.MCPArgs, MCPAllowedTools: c.MCPAllowedTools,
		MCPTimeout: c.MCPTimeout, MCPBackend: c.MCPBackend,
		LiteLLMURL: c.LiteLLMURL, LiteLLMAPIKey: c.LiteLLMAPIKey,
		MCPContextSecret: c.MCPContextSecret, LiteLLMServers: c.LiteLLMServers,
		TurnTimeout: c.TurnTimeout,
	}
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
