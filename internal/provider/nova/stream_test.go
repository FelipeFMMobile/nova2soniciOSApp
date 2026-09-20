package nova

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/provider"
)

func TestLiteLLMHandshakeHeadersToolsAndEvents(t *testing.T) {
	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/realtime" || r.URL.Query().Get("model") != "nova-sonic" || r.Header.Get("Authorization") != "Bearer service-key" || r.Header.Get("X-LiteLLM-Session-ID") != "session-123" || r.Header.Get("X-LiteLLM-Trace-ID") != "request-456" || r.Header.Get("X-LiteLLM-Tags") != "sts,voice,nova" {
			t.Errorf("unexpected request: %s %s %#v", r.URL.Path, r.URL.RawQuery, r.Header)
			return
		}
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteJSON(map[string]any{"type": "session.created", "session": map[string]any{}})
		var update map[string]any
		if c.ReadJSON(&update) != nil {
			return
		}
		received <- update
		_ = c.WriteJSON(map[string]any{"type": "session.updated", "session": map[string]any{}})
		var audio map[string]any
		if c.ReadJSON(&audio) != nil || audio["type"] != "input_audio_buffer.append" {
			return
		}
		_ = c.WriteJSON(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user-1", "transcript": "olá"})
		_ = c.WriteJSON(map[string]any{"type": "response.audio.delta", "item_id": "answer-1", "content_index": 0, "delta": "AAAAAA=="})
		_ = c.WriteJSON(map[string]any{"type": "response.function_call_arguments.done", "item_id": "tool-item", "call_id": "call-1", "name": "notes_list", "arguments": `{}`})
		var result map[string]any
		if c.ReadJSON(&result) == nil {
			received <- result
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := Open(ctx, server.URL, "service-key", "nova-sonic", "session-123", "request-456", []HistoryMessage{{Role: "USER", Content: "antes"}}, []provider.ToolSpec{{Name: "notes_list", Description: "lista", Parameters: []byte(`{"type":"object"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	update := <-received
	session := update["session"].(map[string]any)
	turnDetection, _ := session["turn_detection"].(map[string]any)
	if !strings.Contains(session["instructions"].(string), "antes") || len(session["tools"].([]any)) != 1 || turnDetection["type"] != "server_vad" || session["input_sample_rate_hertz"] != float64(16000) || session["output_sample_rate_hertz"] != float64(24000) {
		t.Fatal(update)
	}
	if err := s.SendAudio("AAAAAA=="); err != nil {
		t.Fatal(err)
	}
	var sawAudio, sawTool bool
	deadline := time.After(time.Second)
	for !sawAudio || !sawTool {
		select {
		case event := <-s.Events():
			sawAudio = sawAudio || event.Kind == "audioOutput"
			if event.Kind == "toolUse" {
				sawTool = event.ToolID == "call-1" && event.ToolName == "notes_list"
			}
		case <-deadline:
			t.Fatal("missing translated events")
		}
	}
	if err := s.SendToolResult("call-1", map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	result := <-received
	if result["type"] != "conversation.item.create" {
		t.Fatal(result)
	}
}

func TestLiteLLMStartupRejectionIsRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteJSON(map[string]any{"type": "session.created"})
		var update any
		_ = c.ReadJSON(&update)
		_ = c.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"message": "secret details"}})
	}))
	defer server.Close()
	if _, err := Open(context.Background(), server.URL, "key", "nova-sonic", "session", "request", nil, nil); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

func TestRealtimeURLValidation(t *testing.T) {
	if got, err := realtimeURL("https://example.test/base", "nova sonic"); err != nil || got != "wss://example.test/base/v1/realtime?model=nova+sonic" {
		t.Fatal(got, err)
	}
	if _, err := realtimeURL("file:///tmp/socket", "nova"); err == nil {
		t.Fatal("invalid scheme accepted")
	}
}
