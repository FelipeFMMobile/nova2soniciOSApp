package gateway

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"os"
	"path/filepath"
	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/orchestrator"
	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/storage"
	"sync/atomic"
	"testing"
	"time"
)

type testTools struct {
	calls    atomic.Int64
	approved atomic.Bool
	delay    time.Duration
}

func (b *testTools) Tools(context.Context) ([]mcp.Tool, error) { return mcp.NotesTools(), nil }
func (b *testTools) Call(ctx context.Context, name string, _ json.RawMessage, _ string, confirmed bool) (mcp.Result, error) {
	b.calls.Add(1)
	b.approved.Store(confirmed)
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			return mcp.Result{}, ctx.Err()
		}
	}
	return mcp.TextResult(map[string]any{"status": "ok", "tool": name, "value": "exclusive 7419"}, false), nil
}
func nativeTool(c *websocket.Conn, id, name, args string) {
	_ = c.WriteJSON(map[string]any{"type": "response.function_call_arguments.done", "item_id": "item-" + id, "call_id": id, "name": name, "arguments": args})
}
func waitTool(c *websocket.Conn) map[string]any {
	for {
		var value map[string]any
		if c.ReadJSON(&value) != nil {
			return nil
		}
		if value["type"] == "conversation.item.create" {
			return value
		}
	}
}
func userPhrase(c *websocket.Conn, id, text string) {
	_ = c.WriteJSON(map[string]any{"type": "conversation.item.input_audio_transcription.delta", "item_id": id, "delta": text})
	_ = c.WriteJSON(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": id, "transcript": text})
}
func TestNovaDiscoversAndExecutesMCP(t *testing.T) {
	seen := make(chan map[string]any, 1)
	url := bridge(t, func(c *websocket.Conn, start map[string]any) {
		seen <- start
		var input map[string]any
		if c.ReadJSON(&input) != nil {
			return
		}
		nativeUser(c, "user-1")
		nativeTool(c, "tool-1", "notes_list", `{"query":"7419"}`)
		result := waitTool(c)
		if result == nil {
			return
		}
		nativeAudio(c, "answer-1")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "answer-1", "stopReason": "END_TURN"})
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel = url, "service-key", "nova-sonic"
	cfg.MCPAllowedTools = []string{"notes.list"}
	s, _, ws := setup(t, cfg)
	backend := &testTools{}
	s.toolFactory = func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil }
	c := dial(t, ws, "")
	sid := startNova(t, c)
	start := <-seen
	session, _ := start["session"].(map[string]any)
	tools, ok := session["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatal("tools not advertised", start)
	}
	send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: "input-1", Sequence: 1, SampleRate: 16000, Audio: "AAAAAA=="})
	until(t, c, protocol.ToolStarted)
	result := until(t, c, protocol.ToolResult)
	if result.Tool.OperationID != "tool-1" || result.Tool.Name != "notes.list" || backend.calls.Load() != 1 {
		t.Fatal(result, backend.calls.Load())
	}
	until(t, c, protocol.TurnCompleted)
}
func TestNovaDeletionRequiresLaterFinalUserConfirmation(t *testing.T) {
	pending := make(chan struct{}, 1)
	proceed := make(chan struct{})
	url := bridge(t, func(c *websocket.Conn, _ map[string]any) {
		var input map[string]any
		if c.ReadJSON(&input) != nil {
			return
		}
		userPhrase(c, "user-1", "exclua a nota")
		nativeTool(c, "delete-1", "notes_delete", `{"id":"note-1"}`)
		if waitTool(c) == nil {
			return
		}
		pending <- struct{}{}
		nativeAudio(c, "answer-1")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "answer-1", "stopReason": "END_TURN"})
		<-proceed
		userPhrase(c, "user-2", "Confirmo excluir.")
		nativeTool(c, "delete-2", "notes_delete", `{"id":"note-1"}`)
		if waitTool(c) == nil {
			return
		}
		nativeAudio(c, "answer-2")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "answer-2", "stopReason": "END_TURN"})
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel = url, "service-key", "nova-sonic"
	cfg.MCPAllowedTools = []string{"notes.delete"}
	s, _, ws := setup(t, cfg)
	backend := &testTools{}
	s.toolFactory = func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil }
	c := dial(t, ws, "")
	sid := startNova(t, c)
	send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: "input-1", Sequence: 1, SampleRate: 16000, Audio: "AAAAAA=="})
	until(t, c, protocol.ToolConfirmation)
	<-pending
	if backend.calls.Load() != 0 {
		t.Fatal("deleted before confirmation")
	}
	until(t, c, protocol.TurnCompleted)
	close(proceed)
	until(t, c, protocol.ToolResult)
	until(t, c, protocol.TurnCompleted)
	if backend.calls.Load() != 1 || !backend.approved.Load() {
		t.Fatal("confirmed deletion not executed")
	}
}
func TestNovaToolTimeoutReturnsFailure(t *testing.T) {
	url := bridge(t, func(c *websocket.Conn, _ map[string]any) {
		var input map[string]any
		if c.ReadJSON(&input) != nil {
			return
		}
		nativeUser(c, "u-1")
		nativeTool(c, "tool-1", "notes_list", `{}`)
		if waitTool(c) == nil {
			return
		}
		nativeAudio(c, "a-1")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "a-1", "stopReason": "END_TURN"})
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel = url, "service-key", "nova-sonic"
	cfg.MCPAllowedTools = []string{"notes.list"}
	cfg.MCPTimeout = 20 * time.Millisecond
	s, _, ws := setup(t, cfg)
	backend := &testTools{delay: time.Second}
	s.toolFactory = func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil }
	c := dial(t, ws, "")
	sid := startNova(t, c)
	send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: "input-1", Sequence: 1, SampleRate: 16000, Audio: "AAAAAA=="})
	event := until(t, c, protocol.ToolResult)
	var r mcp.Result
	if json.Unmarshal(event.Tool.Result, &r) != nil || !r.IsError {
		t.Fatal("timeout claimed success", event)
	}
	until(t, c, protocol.TurnCompleted)
}

func TestGatewayMCPHelper(t *testing.T) {
	if os.Getenv("STS_GATEWAY_MCP_HELPER") != "1" {
		return
	}
	s, err := storage.Open(os.Getenv("STS_GATEWAY_MCP_DB"))
	if err != nil {
		os.Exit(2)
	}
	err = mcp.Serve(context.Background(), os.Stdin, os.Stdout, s)
	s.Close()
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestGatewayRealStdioSQLiteRetryAcrossSessions(t *testing.T) {
	db := filepath.Join(t.TempDir(), "notes.sqlite")
	t.Setenv("STS_GATEWAY_MCP_HELPER", "1")
	t.Setenv("STS_GATEWAY_MCP_DB", db)
	url := bridge(t, func(c *websocket.Conn, start map[string]any) {
		session, _ := start["session"].(map[string]any)
		tools, ok := session["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Error("no MCP tools advertised")
			return
		}
		function, _ := tools[0].(map[string]any)["function"].(map[string]any)
		if function["name"] != "notes_create" || function["parameters"] == nil {
			t.Error("OpenAI function schema missing", tools[0])
			return
		}
		var input map[string]any
		if c.ReadJSON(&input) != nil {
			return
		}
		nativeUser(c, "user-1")
		nativeTool(c, "create-1", "notes_create", `{"title":"Durável","content":"valor único 8426"}`)
		if waitTool(c) == nil {
			return
		}
		nativeAudio(c, "answer-1")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "answer-1", "stopReason": "END_TURN"})
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel = url, "service-key", "nova-sonic"
	cfg.MCPCommand = os.Args[0]
	cfg.MCPArgs = []string{"-test.run=^TestGatewayMCPHelper$"}
	cfg.MCPAllowedTools = []string{"notes.create"}
	_, _, ws := setup(t, cfg)
	var original string
	for i := 0; i < 2; i++ {
		c := dial(t, ws, "")
		send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "nova", RequestID: "same-logical-request"})
		sid := until(t, c, protocol.SessionReady).SessionID
		send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: "input-1", Sequence: 1, SampleRate: 16000, Audio: "AAAAAA=="})
		e := until(t, c, protocol.ToolResult)
		var result mcp.Result
		if json.Unmarshal(e.Tool.Result, &result) != nil || result.IsError {
			t.Fatal(e)
		}
		if i == 0 {
			original = result.Content[0].Text
		} else if result.Content[0].Text != original {
			t.Fatal("retry did not replay durable result")
		}
		until(t, c, protocol.TurnCompleted)
		send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
		until(t, c, protocol.SessionStopped)
		c.Close()
	}
	s, err := storage.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	notes, err := s.List(context.Background(), "8426")
	if err != nil || len(notes) != 1 {
		t.Fatal(notes, err)
	}
}

func TestMicrophoneContinuesWhileMCPRuns(t *testing.T) {
	forwarded := make(chan struct{}, 1)
	url := bridge(t, func(c *websocket.Conn, _ map[string]any) {
		var input map[string]any
		if c.ReadJSON(&input) != nil {
			return
		}
		nativeUser(c, "u-1")
		nativeTool(c, "tool-1", "notes_list", `{}`)
		for c.ReadJSON(&input) == nil {
			if input["type"] == "input_audio_buffer.append" {
				forwarded <- struct{}{}
				break
			}
		}
		if waitTool(c) == nil {
			return
		}
		nativeAudio(c, "a-1")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "a-1", "stopReason": "END_TURN"})
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel = url, "service-key", "nova-sonic"
	cfg.MCPAllowedTools = []string{"notes.list"}
	s, _, ws := setup(t, cfg)
	backend := &testTools{delay: 200 * time.Millisecond}
	s.toolFactory = func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil }
	c := dial(t, ws, "")
	sid := startNova(t, c)
	for sequence := uint64(1); sequence <= 2; sequence++ {
		send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: "input-1", Sequence: sequence, SampleRate: 16000, Audio: "AAAAAA=="})
		if sequence == 1 {
			until(t, c, protocol.ToolStarted)
		}
	}
	select {
	case <-forwarded:
	case <-time.After(time.Second):
		t.Fatal("MCP blocked microphone forwarding")
	}
	until(t, c, protocol.ToolResult)
	until(t, c, protocol.TurnCompleted)
}
