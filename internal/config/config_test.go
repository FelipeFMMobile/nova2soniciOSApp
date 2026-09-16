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
