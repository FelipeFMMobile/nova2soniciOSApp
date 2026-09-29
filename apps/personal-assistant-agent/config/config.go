// Package config defines configuration owned by the personal assistant agent.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"stsmodel.local/mcpwire"
)

const MaxPromptBytes = 64 * 1024

type ServerConfig struct {
	Cwd          string                `json:"cwd,omitempty"`
	Env          map[string]string     `json:"env,omitempty"`
	Alias        string                `json:"alias"`
	Command      string                `json:"command"`
	Args         []string              `json:"args"`
	AllowedTools []string              `json:"allowed_tools"`
	Policies     map[string]mcp.Policy `json:"policies"`
}

type GatewayServerConfig struct {
	Alias        string                `json:"alias"`
	ServerID     string                `json:"server_id"`
	AllowedTools []string              `json:"allowed_tools"`
	Policies     map[string]mcp.Policy `json:"policies"`
}

type Config struct {
	Provider          string
	PromptFile        string
	NovaMaxSessionAge time.Duration
	LiteLLMModel      string
	MCPServers        []ServerConfig
	MCPEvidencePath   string
	MCPCommand        string
	MCPArgs           []string
	MCPAllowedTools   []string
	MCPTimeout        time.Duration
	MCPBackend        string
	LiteLLMURL        string
	LiteLLMAPIKey     string
	MCPContextSecret  string
	LiteLLMServers    []GatewayServerConfig
	TurnTimeout       time.Duration
}

func LoadPrompt(path string) (string, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return "", errors.New("agent prompt file must be an absolute path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("agent prompt file is unavailable")
	}
	if info.Size() > MaxPromptBytes {
		return "", errors.New("agent prompt exceeds 64 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("agent prompt file cannot be read")
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", errors.New("agent prompt file is empty")
	}
	return prompt, nil
}

func (c Config) Validate() error {
	if c.Provider != "nova" {
		return fmt.Errorf("personal assistant only supports provider %q", c.Provider)
	}
	if c.NovaMaxSessionAge <= 0 || c.NovaMaxSessionAge > 7*time.Minute+30*time.Second || c.MCPTimeout <= 0 || c.TurnTimeout <= 0 {
		return errors.New("agent timeouts must be positive and session age at most 7m30s")
	}
	if strings.TrimSpace(c.LiteLLMModel) == "" || c.LiteLLMAPIKey == "" {
		return errors.New("agent requires LiteLLM API key and realtime model")
	}
	parsed, err := url.Parse(c.LiteLLMURL)
	if err != nil || parsed.Host == "" || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("agent LiteLLM URL must be an HTTP(S) base URL")
	}
	if c.MCPBackend != "stdio" && c.MCPBackend != "litellm" {
		return errors.New("agent MCP backend must be stdio or litellm")
	}
	_, err = LoadPrompt(c.PromptFile)
	return err
}
