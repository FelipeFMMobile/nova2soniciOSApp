package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"stsmodel.local/agentwire"
)

type Service struct {
	Runtime      Config
	Address      string
	AgentID      string
	Token        string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxSessions  int
}

func LoadService() (Service, error) {
	duration := func(key string, fallback time.Duration) (time.Duration, error) {
		raw := os.Getenv(key)
		if raw == "" {
			return fallback, nil
		}
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return 0, fmt.Errorf("%s must be a positive duration", key)
		}
		return value, nil
	}
	readTimeout, err := duration("STS_AGENT_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		return Service{}, err
	}
	writeTimeout, err := duration("STS_AGENT_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Service{}, err
	}
	idleTimeout, err := duration("STS_AGENT_IDLE_TIMEOUT", 90*time.Second)
	if err != nil {
		return Service{}, err
	}
	turnTimeout, err := duration("STS_TURN_TIMEOUT", 30*time.Second)
	if err != nil {
		return Service{}, err
	}
	sessionAge, err := duration("STS_NOVA_MAX_SESSION_AGE", 7*time.Minute+30*time.Second)
	if err != nil {
		return Service{}, err
	}
	mcpTimeout, err := duration("STS_MCP_TIMEOUT", 10*time.Second)
	if err != nil {
		return Service{}, err
	}
	maxSessions := 16
	if raw := os.Getenv("STS_AGENT_MAX_SESSIONS"); raw != "" {
		maxSessions, err = strconv.Atoi(raw)
		if err != nil || maxSessions <= 0 || maxSessions > agentwire.MaxRemoteSessions {
			return Service{}, errors.New("STS_AGENT_MAX_SESSIONS is invalid")
		}
	}
	service := Service{
		Address: envValue("STS_AGENT_ADDRESS", "127.0.0.1:8090"),
		AgentID: envValue("STS_AGENT_ID", "personal-assistant"),
		Token:   os.Getenv("STS_AGENT_TOKEN"), ReadTimeout: readTimeout,
		WriteTimeout: writeTimeout, IdleTimeout: idleTimeout, MaxSessions: maxSessions,
		Runtime: Config{
			Provider: "nova", PromptFile: os.Getenv("STS_AGENT_PROMPT_FILE"),
			NovaMaxSessionAge: sessionAge, LiteLLMModel: envValue("STS_LITELLM_REALTIME_MODEL", "nova-sonic"),
			MCPEvidencePath: os.Getenv("STS_MCP_EVIDENCE_PATH"), MCPCommand: os.Getenv("STS_MCP_COMMAND"),
			MCPAllowedTools: splitCSV(envValue("STS_MCP_ALLOWED_TOOLS", "notes.create,notes.list,notes.delete")),
			MCPTimeout:      mcpTimeout, MCPBackend: envValue("STS_MCP_BACKEND", "stdio"),
			LiteLLMURL: os.Getenv("STS_LITELLM_URL"), LiteLLMAPIKey: os.Getenv("STS_LITELLM_API_KEY"),
			MCPContextSecret: os.Getenv("STS_MCP_CONTEXT_SECRET"), TurnTimeout: turnTimeout,
		},
	}
	if !agentwire.IsLoopbackAddress(service.Address) {
		return Service{}, errors.New("Agent Session v1alpha1 must listen on loopback")
	}
	if service.Token == "" || len(service.Token) > 4096 {
		return Service{}, errors.New("STS_AGENT_TOKEN is required")
	}
	if raw := os.Getenv("STS_MCP_ARGS"); raw != "" {
		if json.Unmarshal([]byte(raw), &service.Runtime.MCPArgs) != nil {
			return Service{}, errors.New("STS_MCP_ARGS must be a JSON string array")
		}
	}
	if raw := os.Getenv("STS_MCP_SERVERS"); raw != "" {
		if service.Runtime.MCPCommand != "" || json.Unmarshal([]byte(raw), &service.Runtime.MCPServers) != nil || len(service.Runtime.MCPServers) == 0 {
			return Service{}, errors.New("STS_MCP_SERVERS must be a nonempty unambiguous JSON server array")
		}
	}
	if service.Runtime.MCPCommand != "" && !filepath.IsAbs(service.Runtime.MCPCommand) {
		return Service{}, errors.New("STS_MCP_COMMAND must be absolute")
	}
	if service.Runtime.MCPBackend == "litellm" {
		if service.Runtime.MCPCommand != "" || len(service.Runtime.MCPServers) != 0 || json.Unmarshal([]byte(os.Getenv("STS_LITELLM_MCP_SERVERS")), &service.Runtime.LiteLLMServers) != nil {
			return Service{}, errors.New("invalid LiteLLM MCP configuration")
		}
	}
	if err := service.Runtime.Validate(); err != nil {
		return Service{}, err
	}
	return service, nil
}

func envValue(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitCSV(raw string) []string {
	values := strings.Split(raw, ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return values
}
