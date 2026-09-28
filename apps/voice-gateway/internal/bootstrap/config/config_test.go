package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("STS_ENV", "development")
	t.Setenv("STS_PROVIDER", "fake")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GatewayAddress != "127.0.0.1:8080" {
		t.Fatalf("GatewayAddress = %q", cfg.GatewayAddress)
	}
	if cfg.MaxEventBytes != 512*1024 {
		t.Fatalf("MaxEventBytes = %d", cfg.MaxEventBytes)
	}
}

func TestRejectsUnsupportedProvider(t *testing.T) {
	t.Setenv("STS_ENV", "development")
	t.Setenv("STS_PROVIDER", "qwen")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestNovaRequiresLiteLLM(t *testing.T) {
	t.Setenv("STS_ENV", "development")
	t.Setenv("STS_PROVIDER", "nova")
	t.Setenv("STS_LITELLM_URL", "")
	t.Setenv("STS_LITELLM_API_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("Nova without LiteLLM configuration was accepted")
	}
	t.Setenv("STS_LITELLM_URL", "http://127.0.0.1:4000")
	t.Setenv("STS_LITELLM_API_KEY", "service-key")
	cfg, err := Load()
	if err != nil || cfg.LiteLLMModel != "nova-sonic" {
		t.Fatal(cfg, err)
	}
}

func TestNovaRejectsInvalidLiteLLMFields(t *testing.T) {
	for _, tc := range []struct{ name, url, key, model string }{
		{"missing URL", "", "key", "nova-sonic"},
		{"invalid URL", "file:///tmp/litellm", "key", "nova-sonic"},
		{"missing key", "http://127.0.0.1:4000", "", "nova-sonic"},
		{"blank model", "http://127.0.0.1:4000", "key", " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STS_PROVIDER", "nova")
			t.Setenv("STS_LITELLM_URL", tc.url)
			t.Setenv("STS_LITELLM_API_KEY", tc.key)
			t.Setenv("STS_LITELLM_REALTIME_MODEL", tc.model)
			if _, err := Load(); err == nil {
				t.Fatal("invalid LiteLLM configuration accepted")
			}
		})
	}
}

func TestRejectsMalformedLimits(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"STS_READ_TIMEOUT", "oops"}, {"STS_WRITE_TIMEOUT", "0s"}, {"STS_IDLE_TIMEOUT", "-1s"}, {"STS_TURN_TIMEOUT", "-5s"}, {"STS_MAX_SESSIONS", "0"}, {"STS_MAX_EVENT_BYTES", "oops"}} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("malformed limit accepted")
			}
		})
	}
}

func TestNonLoopbackRequiresToken(t *testing.T) {
	t.Setenv("STS_ENV", "development")
	t.Setenv("STS_PROVIDER", "fake")
	t.Setenv("STS_GATEWAY_ADDRESS", "0.0.0.0:8080")
	t.Setenv("STS_DEVELOPMENT_TOKEN", "")
	if _, err := Load(); err == nil {
		t.Fatal("unauthenticated network listener accepted")
	}
	t.Setenv("STS_DEVELOPMENT_TOKEN", "test-token")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestNovaSessionAgeCannotReachBedrockHardLimit(t *testing.T) {
	t.Setenv("STS_NOVA_MAX_SESSION_AGE", "8m")
	if _, err := Load(); err == nil {
		t.Fatal("unsafe renewal deadline accepted")
	}
}

func TestMCPConfigurationIsNotShellCode(t *testing.T) {
	t.Setenv("STS_MCP_COMMAND", "/absolute/bin/mcp-notes")
	t.Setenv("STS_MCP_ARGS", `["-db","a path/sts.sqlite"]`)
	cfg, err := Load()
	if err != nil || len(cfg.MCPArgs) != 2 || cfg.MCPArgs[1] != "a path/sts.sqlite" {
		t.Fatal(cfg, err)
	}
	t.Setenv("STS_MCP_ARGS", "-db /tmp/a; echo secret")
	if _, err = Load(); err == nil {
		t.Fatal("shell command accepted as args")
	}
	t.Setenv("STS_MCP_ARGS", "[]")
	t.Setenv("STS_MCP_COMMAND", "mcp-notes")
	if _, err = Load(); err == nil {
		t.Fatal("relative executable accepted")
	}
}

func TestExplicitMultipleServers(t *testing.T) {
	t.Setenv("STS_MCP_COMMAND", "")
	t.Setenv("STS_MCP_SERVERS", `[{"alias":"local","command":"/absolute/mcp-agenda","args":["-db","a path/agenda.sqlite"],"allowed_tools":["agenda.list_events"],"policies":{"agenda.list_events":"read_only"}}]`)
	cfg, err := Load()
	if err != nil || len(cfg.MCPServers) != 1 || cfg.MCPServers[0].Args[1] != "a path/agenda.sqlite" {
		t.Fatal(cfg, err)
	}
	t.Setenv("STS_MCP_COMMAND", "/absolute/mcp-notes")
	if _, err = Load(); err == nil {
		t.Fatal("ambiguous configuration")
	}
	t.Setenv("STS_MCP_COMMAND", "")
	t.Setenv("STS_MCP_SERVERS", `[{"alias":"a","command":"/bin/a","allowed_tools":["x"]}]`)
	if _, err = Load(); err == nil {
		t.Fatal("missing host policy")
	}
}

func TestLiteLLMGatewayConfiguration(t *testing.T) {
	t.Setenv("STS_MCP_BACKEND", "litellm")
	t.Setenv("STS_LITELLM_URL", "http://127.0.0.1:4000")
	t.Setenv("STS_LITELLM_API_KEY", "service-key")
	t.Setenv("STS_MCP_CONTEXT_SECRET", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	t.Setenv("STS_LITELLM_MCP_SERVERS", `[{"alias":"memo","server_id":"notes","allowed_tools":["notes.list"],"policies":{"notes.list":"read_only"}}]`)
	cfg, err := Load()
	if err != nil || len(cfg.LiteLLMServers) != 1 {
		t.Fatal(cfg, err)
	}
	t.Setenv("STS_MCP_COMMAND", "/bin/mcp")
	if _, err = Load(); err == nil {
		t.Fatal("mixed backends accepted")
	}
}
