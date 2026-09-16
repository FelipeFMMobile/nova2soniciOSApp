package main

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stsmodel.local/poc/internal/audio"
	"stsmodel.local/poc/internal/config"
	"stsmodel.local/poc/internal/gateway"
)

func TestTerminalDemoAndCancellation(t *testing.T) {
	t.Setenv("STS_PROVIDER", "fake")
	t.Setenv("STS_DEVELOPMENT_TOKEN", "demo-secret")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	s, err := gateway.New(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s)
	defer h.Close()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	url := "ws" + strings.TrimPrefix(h.URL, "http") + "/v1/voice"
	for _, cancelAfter := range []time.Duration{0, 120 * time.Millisecond} {
		path := filepath.Join(t.TempDir(), "response.wav")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := run(ctx, url, "", path, cancelAfter)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if cancelAfter > 0 {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("cancelled audio was saved")
			}
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		pcm, err := audio.ReadPCM16WAV(file, 24000, 48000)
		file.Close()
		if err != nil || len(pcm) != 38400 {
			t.Fatal("incomplete WAV output", len(pcm), err)
		}
	}
}
