package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	orchestrator "stsmodel.local/actionruntime"
	mcp "stsmodel.local/mcpwire"
	"stsmodel.local/voicewire"
)

type welcomeBackend struct{ calls atomic.Int32 }

func (b *welcomeBackend) Tools(context.Context) ([]mcp.Tool, error) {
	return []mcp.Tool{{Name: "identity.begin_login", Description: "Abrir login após consentimento explícito.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}, nil
}
func (b *welcomeBackend) Call(context.Context, string, json.RawMessage, string, bool) (mcp.Result, error) {
	b.calls.Add(1)
	return mcp.TextResult(map[string]string{"transaction_id": strings.Repeat("a", 43)}, false), nil
}

func TestRuntimeVoiceInvitation(t *testing.T) {
	for _, tc := range []struct {
		answer        string
		wantUI        bool
		authenticated bool
	}{{"sim", true, false}, {"não", false, false}, {"após login", false, true}, {"após restauração", false, true}} {
		t.Run(tc.answer, func(t *testing.T) {
			request, messageText, turn := welcomeRequest, welcomeMessage, welcomeOperation
			if tc.authenticated {
				request, messageText, turn = connectedRequest, connectedMessage, connectedTurn
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			invitation := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer c.Close()
				_ = c.WriteJSON(map[string]any{"type": "session.created"})
				var message map[string]any
				if c.ReadJSON(&message) != nil {
					return
				}
				_ = c.WriteJSON(map[string]any{"type": "session.updated"})
				if c.ReadJSON(&message) != nil || message["type"] != "conversation.item.create" {
					t.Error("agent did not request provider greeting")
					return
				}
				item, _ := message["item"].(map[string]any)
				content, _ := item["content"].([]any)
				if len(content) != 1 || content[0].(map[string]any)["text"] != request {
					t.Error("wrong opening request")
					return
				}
				if c.ReadJSON(&message) != nil || message["type"] != "response.create" {
					t.Error("missing greeting response request")
					return
				}
				select {
				case <-invitation:
				case <-ctx.Done():
					return
				}
				_ = c.WriteJSON(map[string]any{"type": "input_audio_buffer.speech_started"})
				_ = c.WriteJSON(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "bootstrap", "transcript": "ready"})
				_ = c.WriteJSON(map[string]any{"type": "response.audio.delta", "item_id": "greeting-audio", "delta": "AAAAAA=="})
				_ = c.WriteJSON(map[string]any{"type": "response.text.delta", "item_id": "greeting-text", "delta": messageText})
				_ = c.WriteJSON(map[string]any{"type": "response.text.done", "item_id": "greeting-text"})
				_ = c.WriteJSON(map[string]any{"type": "response.audio.done", "item_id": "greeting-audio"})
				_ = c.WriteJSON(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user-1", "transcript": tc.answer})
				_ = c.WriteJSON(map[string]any{"type": "response.content_part.added", "item_id": "assistant-1", "content_index": 0, "part": map[string]any{"type": "text"}})
				for c.ReadJSON(&message) == nil {
				}
			}))
			defer server.Close()
			backend := &welcomeBackend{}
			runtime := &Runtime{authenticated: tc.authenticated, cfg: Config{LiteLLMURL: server.URL, LiteLLMAPIKey: "synthetic", LiteLLMModel: "test", NovaMaxSessionAge: time.Minute, MCPTimeout: time.Second, TurnTimeout: time.Second}, prompt: "test", toolFactory: func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil }}
			sawUI, sawClosed, sawAudio := false, false, false
			sawBootstrap, sawConnected := false, false
			runtime.Run(ctx, make(chan Input), "voice-welcome", "request-welcome", func(event protocol.Event) error {
				if tc.authenticated && event.Type == protocol.SessionReady {
					close(invitation)
				}
				if tc.authenticated && event.Type == protocol.Transcript && event.Role == "ASSISTANT" && event.Text == connectedMessage {
					sawConnected = true
					cancel()
				}
				if event.Type == protocol.Transcript && event.Role == "USER" && event.Text == "ready" {
					sawBootstrap = true
				}
				if event.Type == protocol.AudioOutput && event.TurnID == turn {
					sawAudio = true
				}
				if event.Type == protocol.ToolConfirmation && event.Tool.OperationID == welcomeOperation {
					close(invitation)
				}
				if event.Type == "ui.a2ui" {
					sawUI = true
					cancel()
				}
				if event.Type == protocol.SessionStopped {
					sawClosed = true
				}
				return nil
			})
			if sawBootstrap {
				t.Fatal("bootstrap treated as user speech")
			}
			if !sawAudio {
				t.Fatal("provider greeting audio did not reach the app")
			}
			if tc.authenticated {
				if !sawConnected || sawUI || backend.calls.Load() != 0 {
					t.Fatal("authenticated opening did not confirm connection cleanly")
				}
				return
			}
			if sawUI != tc.wantUI || sawClosed == tc.wantUI {
				t.Fatalf("UI=%v closed=%v; expected UI=%v", sawUI, sawClosed, tc.wantUI)
			}
			wantCalls := int32(0)
			if tc.wantUI {
				wantCalls = 1
			}
			if backend.calls.Load() != wantCalls {
				t.Fatalf("login calls=%d; want %d", backend.calls.Load(), wantCalls)
			}
		})
	}
}

func TestLiveNovaOpeningWithSilentMicrophone(t *testing.T)   { testLiveNovaOpening(t, false) }
func TestLiveNovaConnectedWithSilentMicrophone(t *testing.T) { testLiveNovaOpening(t, true) }
func testLiveNovaOpening(t *testing.T, authenticated bool) {
	if os.Getenv("STS_NOVA_GREETING_TEST") != "1" {
		t.Skip("requires opt-in Nova endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cfg := Config{LiteLLMURL: os.Getenv("STS_LITELLM_URL"), LiteLLMAPIKey: os.Getenv("STS_LITELLM_API_KEY"), LiteLLMModel: "nova-sonic", PromptFile: "../../prompts/system.md", NovaMaxSessionAge: time.Minute, MCPTimeout: time.Second, TurnTimeout: 20 * time.Second}
	backend := &welcomeBackend{}
	runtime, err := New(cfg, nil, Hooks{}, func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	runtime.authenticated = authenticated
	input := make(chan Input, 8)
	go func() {
		ticker := time.NewTicker(32 * time.Millisecond)
		defer ticker.Stop()
		var seq uint64
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				seq++
				select {
				case input <- Input{Event: protocol.Event{Type: protocol.AudioAppend, SessionID: "opening-test", TurnID: "microphone", Sequence: seq, Audio: base64.StdEncoding.EncodeToString(make([]byte, 1024))}}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	audio := 0
	text := ""
	completed := false
	internalTranscript := false
	runtime.Run(ctx, input, "opening-test", "opening-test", func(e protocol.Event) error {
		if e.Type == protocol.Error {
			t.Errorf("runtime error code: %s", e.Code)
			cancel()
		}
		if e.Type == protocol.AudioOutput {
			audio++
		}
		if e.Type == protocol.Transcript && e.Role == "ASSISTANT" && e.Stage == "FINAL" {
			text += e.Text
		}
		if e.Type == protocol.Transcript && e.Role == "USER" {
			internalTranscript = true
		}
		if e.Type == protocol.TurnCompleted && audio > 0 {
			completed = true
			cancel()
		}
		return nil
	})
	expected := "continuar"
	if authenticated {
		expected = "conta conectada com sucesso"
	}
	if !completed || audio == 0 || !strings.Contains(strings.ToLower(text), expected) {
		t.Fatalf("greeting incomplete; audio frames=%d, transcript bytes=%d", audio, len(text))
	}
	if backend.calls.Load() != 0 {
		t.Fatal("login ran without consent")
	}
	if internalTranscript {
		t.Fatal("transport bootstrap was exposed as user speech")
	}
	t.Logf("Agent delivered %d Nova audio frames; login calls=0; no fabricated user transcript", audio)
}
