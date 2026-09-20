package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

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

func TestTerminalNativeBargeInRetainsOnlyNewResponse(t *testing.T) {
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		_ = c.WriteJSON(map[string]any{"type": "session.created", "session": map[string]any{}})
		var request map[string]any
		if c.ReadJSON(&request) != nil {
			return
		}
		_ = c.WriteJSON(map[string]any{"type": "session.updated", "session": map[string]any{}})
		start := func(id string) {
			_ = c.WriteJSON(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "u-" + id, "transcript": "Fala"})
			_ = c.WriteJSON(map[string]any{"type": "response.content_part.added", "item_id": "a-" + id, "content_index": 0, "part": map[string]any{"type": "audio"}})
			_ = c.WriteJSON(map[string]any{"type": "response.audio.delta", "item_id": "a-" + id, "content_index": 0, "delta": "AAAAAA=="})
		}
		first := true
		for c.ReadJSON(&request) == nil {
			if request["type"] != "input_audio_buffer.append" {
				continue
			}
			encoded, _ := request["audio"].(string)
			pcm, _ := base64.StdEncoding.DecodeString(encoded)
			if first {
				first = false
				start("first")
				continue
			}
			if len(pcm) > 0 && pcm[0] != 0 {
				_ = c.WriteJSON(map[string]any{"type": "input_audio_buffer.speech_started", "item_id": "u-second"})
				start("second")
				_ = c.WriteJSON(map[string]any{"type": "response.audio.done", "item_id": "a-second", "content_index": 0})
				_ = c.WriteJSON(map[string]any{"type": "response.done", "response": map[string]any{"status": "completed"}})
				break
			}
		}
		for c.ReadJSON(&request) == nil {
		}
	}))
	defer bridge.Close()
	t.Setenv("STS_PROVIDER", "nova")
	t.Setenv("STS_LITELLM_URL", bridge.URL)
	t.Setenv("STS_LITELLM_API_KEY", "service-key")
	t.Setenv("STS_LITELLM_REALTIME_MODEL", "nova-sonic")
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
		_ = s.Close(ctx)
	}()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.wav")
	barge := filepath.Join(dir, "barge.wav")
	output := filepath.Join(dir, "output.wav")
	for path, pcm := range map[string][]byte{input: make([]byte, 1024), barge: {1, 0, 1, 0}} {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = audio.WritePCM16WAV(file, pcm, 16000)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runProvider(ctx, "ws"+strings.TrimPrefix(h.URL, "http")+"/v1/voice", input, output, 0, "nova", barge); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	pcm, err := audio.ReadPCM16WAV(file, 24000, 4096)
	if err != nil || len(pcm) != 4 {
		t.Fatal("old audio retained", len(pcm), err)
	}
}
