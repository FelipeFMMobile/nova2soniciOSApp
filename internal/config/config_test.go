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

func TestQwenRequiresEndpointAndKey(t *testing.T) {
	t.Setenv("STS_ENV", "development")
	t.Setenv("STS_PROVIDER", "qwen")
	t.Setenv("STS_QWEN_REALTIME_URL", "")
	t.Setenv("STS_QWEN_API_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}
