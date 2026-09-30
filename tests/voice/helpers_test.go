package gateway

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"stsmodel.local/agentwire"
	config "stsmodel.local/personal-assistant-agent/config"
	assistant "stsmodel.local/personal-assistant-agent/session"
	"stsmodel.local/testkit"
	"stsmodel.local/voicewire"
	"testing"
	"time"
)

type harness struct {
	logger      *slog.Logger
	toolFactory assistant.ToolFactory
}
type factoryFunc func(context.Context, agentwire.AgentSessionStart) (agentwire.AgentSession, error)

func (f factoryFunc) Open(ctx context.Context, s agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
	return f(ctx, s)
}
func testConfig() config.Config {
	return config.Config{Provider: "nova", PromptFile: filepath.Join(testkit.Root(), "apps/personal-assistant-agent/prompts/system.md"), MCPBackend: "stdio", MCPTimeout: 10 * time.Second, NovaMaxSessionAge: 450 * time.Second, TurnTimeout: 2 * time.Second}
}
func setup(t *testing.T, cfg config.Config) (*harness, *testkit.Gateway, string) {
	t.Helper()
	h := &harness{logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	handler, err := agentwire.NewHandler(agentwire.ServerConfig{AgentID: "personal-assistant", Token: "integration-token", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: 30 * time.Second, MaxSessions: 16}, factoryFunc(func(ctx context.Context, start agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
		f, err := assistant.New(cfg, h.logger, assistant.Hooks{}, h.toolFactory)
		if err != nil {
			return nil, err
		}
		return f.Open(ctx, start)
	}))
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	catalog, _ := json.Marshal([]map[string]string{{"id": "personal-assistant", "endpoint": "ws" + strings.TrimPrefix(remote.URL, "http") + "/v1/session", "protocol": agentwire.Protocol, "token_env": "STS_AGENT_TOKEN"}})
	edge := testkit.StartGatewayWithEnv(t, []string{"STS_PROVIDER=nova", "STS_AGENT_TOKEN=integration-token", "STS_AGENT_CATALOG=" + string(catalog)})
	return h, edge, "ws" + strings.TrimPrefix(edge.URL, "http") + "/v1/voice"
}
func dial(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	c, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func send(t *testing.T, c *websocket.Conn, e protocol.Event) {
	t.Helper()
	e.Version = 1
	if err := c.WriteJSON(e); err != nil {
		t.Fatal(err)
	}
}
func receive(t *testing.T, c *websocket.Conn) protocol.Event {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var e protocol.Event
	if err := c.ReadJSON(&e); err != nil {
		t.Fatal(err)
	}
	if e.Version != 1 {
		t.Fatal("wrong version")
	}
	return e
}
func until(t *testing.T, c *websocket.Conn, kind string) protocol.Event {
	t.Helper()
	for i := 0; i < 256; i++ {
		e := receive(t, c)
		if e.Type == kind {
			return e
		}
		if e.Type == protocol.Error {
			t.Fatalf("unexpected error: %+v", e)
		}
	}
	t.Fatal("missing event", kind)
	return protocol.Event{}
}
func start(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "fake"})
	return until(t, c, protocol.SessionReady).SessionID
}
func appendAudio(t *testing.T, c *websocket.Conn, sid, tid string, sequence uint64) {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: tid, Sequence: sequence, SampleRate: 16000, Audio: "AAAAAA=="})
}
func commit(t *testing.T, c *websocket.Conn, sid, tid string) {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.TurnCommit, SessionID: sid, TurnID: tid})
}
