package config

import (
	"path/filepath"
	"testing"
)

func TestLoadServiceRequiresLoopbackTokenAndAgentSecrets(t *testing.T) {
	prompt, err := filepath.Abs("../prompts/system.md")
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"STS_AGENT_TOKEN": "internal-token", "STS_AGENT_PROMPT_FILE": prompt,
		"STS_AGENT_ADDRESS": "127.0.0.1:8090", "STS_MCP_BACKEND": "stdio",
		"STS_LITELLM_URL": "http://127.0.0.1:4000", "STS_LITELLM_API_KEY": "service-key",
	} {
		t.Setenv(key, value)
	}
	service, err := LoadService()
	if err != nil || service.AgentID != "personal-assistant" || service.Address != "127.0.0.1:8090" {
		t.Fatal(service, err)
	}
	t.Setenv("STS_AGENT_ADDRESS", "0.0.0.0:8090")
	if _, err := LoadService(); err == nil {
		t.Fatal("non-loopback listener accepted")
	}
}
