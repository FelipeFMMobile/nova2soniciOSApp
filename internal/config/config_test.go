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

func TestNovaUsesLocalBridgeByDefault(t *testing.T) {
	t.Setenv("STS_ENV", "development")
	t.Setenv("STS_PROVIDER", "nova")
	t.Setenv("STS_NOVA_BRIDGE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.NovaBridgeURL != "ws://127.0.0.1:8091" {
		t.Fatalf("NovaBridgeURL = %q", cfg.NovaBridgeURL)
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
