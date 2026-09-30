package config

import (
	"stsmodel.local/agentwire"
	"testing"
)

func base(t *testing.T) {
	t.Helper()
	t.Setenv("STS_AGENT_MODE", "")
	t.Setenv("STS_AGENT_TOKEN", "test-token")
	t.Setenv("STS_AGENT_CATALOG", `[{"id":"personal-assistant","endpoint":"ws://127.0.0.1:8090/v1/session","protocol":"agent-session/v1","token_env":"STS_AGENT_TOKEN"}]`)
}
func TestLoadTransportDefaults(t *testing.T) {
	base(t)
	cfg, err := Load()
	if err != nil || cfg.GatewayAddress != "127.0.0.1:8080" || cfg.MaxEventBytes != 512*1024 || len(cfg.AgentCatalog) != 1 {
		t.Fatal(cfg, err)
	}
}
func TestSemanticEnvironmentAndLegacyModeRejected(t *testing.T) {
	for _, key := range []string{"STS_LITELLM_API_KEY", "STS_MCP_SERVERS", "STS_NOVA_MAX_SESSION_AGE", "STS_AGENT_PROMPT_FILE", "STS_INTEGRATIONS_DIR", "STS_TURN_TIMEOUT"} {
		t.Run(key, func(t *testing.T) {
			base(t)
			t.Setenv(key, "not-for-edge")
			if _, err := Load(); err == nil {
				t.Fatal("semantic config accepted")
			}
		})
	}
	t.Run("legacy", func(t *testing.T) {
		base(t)
		t.Setenv("STS_AGENT_MODE", "inprocess")
		if _, err := Load(); err == nil {
			t.Fatal("legacy mode accepted")
		}
	})
}
func TestCatalogAndPublicLimitsFailClosed(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"STS_AGENT_CATALOG", "[]"}, {"STS_AGENT_CATALOG", `[{"id":"personal-assistant","endpoint":"ws://example.com/v1/session","protocol":"agent-session/v1","token_env":"STS_AGENT_TOKEN"}]`},
		{"STS_DEFAULT_AGENT_ID", "missing"}, {"STS_AGENT_TOKEN", ""}, {"STS_PROVIDER", "unknown"}, {"STS_READ_TIMEOUT", "oops"}, {"STS_IDLE_TIMEOUT", "0s"}, {"STS_MAX_SESSIONS", "0"}, {"STS_MAX_EVENT_BYTES", "oops"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			base(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
func TestNonLoopbackRequiresToken(t *testing.T) {
	base(t)
	t.Setenv("STS_GATEWAY_ADDRESS", "0.0.0.0:8080")
	t.Setenv("STS_DEVELOPMENT_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("public listener accepted without token")
	}
	t.Setenv("STS_DEVELOPMENT_TOKEN", "edge-token")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
func TestNovaNeedsOnlyAgentCatalog(t *testing.T) {
	base(t)
	t.Setenv("STS_PROVIDER", "nova")
	cfg, err := Load()
	if err != nil || cfg.Provider != "nova" {
		t.Fatal(cfg, err)
	}
	if err := agentwire.ValidateEndpoint(cfg.AgentCatalog[0]); err != nil {
		t.Fatal(err)
	}
}
