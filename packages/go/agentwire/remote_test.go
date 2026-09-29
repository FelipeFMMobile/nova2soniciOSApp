package agentwire

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type recordingFactory struct {
	starts chan AgentSessionStart
}

func (f *recordingFactory) Open(_ context.Context, start AgentSessionStart) (AgentSession, error) {
	s := &recordingSession{events: make(chan AgentEvent, RemoteQueueDepth), commands: make(chan AgentCommand, RemoteQueueDepth)}
	s.events <- AgentEvent{Type: AgentSessionReady, Provider: start.Provider, State: Idle}
	f.starts <- start
	return s, nil
}

type recordingSession struct {
	events   chan AgentEvent
	commands chan AgentCommand
	once     sync.Once
}

func (s *recordingSession) Send(_ context.Context, command AgentCommand) error {
	s.commands <- command
	if command.Type == InputClose {
		s.events <- AgentEvent{Type: AgentSessionClosed, State: Closed}
	}
	return nil
}
func (s *recordingSession) Events() <-chan AgentEvent { return s.events }
func (s *recordingSession) Close() error {
	s.once.Do(func() { close(s.events) })
	return nil
}

func endpointFor(server *httptest.Server, id, token string) Endpoint {
	return Endpoint{AgentID: id, URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/v1alpha1/session", Token: token}
}

func testAgentServer(t *testing.T, id, token string) (*httptest.Server, *recordingFactory) {
	t.Helper()
	factory := &recordingFactory{starts: make(chan AgentSessionStart, 1)}
	handler, err := NewHandler(ServerConfig{AgentID: id, Token: token, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: 3 * time.Second, MaxSessions: 2}, factory)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(handler), factory
}

func TestRemoteFactoryRoutesOnlyThroughTrustedCatalog(t *testing.T) {
	alphaServer, alpha := testAgentServer(t, "alpha", "alpha-token")
	defer alphaServer.Close()
	betaServer, beta := testAgentServer(t, "beta", "beta-token")
	defer betaServer.Close()
	factory, err := NewRemoteFactory([]Endpoint{endpointFor(alphaServer, "alpha", "alpha-token"), endpointFor(betaServer, "beta", "beta-token")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := factory.Open(ctx, AgentSessionStart{SessionID: "session-1", RequestID: "request-1", AgentID: "beta", Provider: "nova"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case start := <-beta.starts:
		if start.AgentID != "beta" || start.SessionID != "session-1" {
			t.Fatal(start)
		}
	case <-ctx.Done():
		t.Fatal("beta agent did not receive session")
	}
	select {
	case start := <-alpha.starts:
		t.Fatal("wrong agent received session", start)
	default:
	}
	select {
	case event := <-session.Events():
		if event.Type != AgentSessionReady {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal("ready event not forwarded")
	}
	if err := session.Send(ctx, AgentCommand{Type: InputAudio, SessionID: "session-1", TurnID: "turn-1", Sequence: 1, Audio: []byte{0, 0}}); err != nil {
		t.Fatal(err)
	}
	if err := session.Send(ctx, AgentCommand{Type: InputClose, SessionID: "session-1"}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-session.Events():
		if event.Type != AgentSessionClosed {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal("close event not forwarded")
	}
}

func TestRemoteFactoryFailsClosed(t *testing.T) {
	server, _ := testAgentServer(t, "personal-assistant", "correct-token")
	defer server.Close()
	factory, err := NewRemoteFactory([]Endpoint{endpointFor(server, "personal-assistant", "wrong-token")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Open(context.Background(), AgentSessionStart{SessionID: "session-1", AgentID: "personal-assistant", Provider: "nova"}); err == nil {
		t.Fatal("invalid credential accepted")
	}
	if _, err := factory.Open(context.Background(), AgentSessionStart{SessionID: "session-1", AgentID: "unknown", Provider: "nova"}); err == nil {
		t.Fatal("unknown agent accepted")
	}
	if err := ValidateEndpoint(Endpoint{AgentID: "remote", URL: "ws://192.0.2.1/session", Token: "token"}); err == nil {
		t.Fatal("unencrypted non-loopback endpoint accepted")
	}
}

func TestRemoteFactoryRejectsIncompatibleHandshake(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseHeaders := http.Header{}
		responseHeaders.Set(proofHeader, serverProof("token", "personal-assistant", r.Header.Get(nonceHeader)))
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, responseHeaders)
		if err != nil {
			return
		}
		defer connection.Close()
		_, _, _ = connection.ReadMessage()
		_ = connection.WriteJSON(map[string]any{"protocol": "agent-session/v2", "kind": SessionAccepted, "agentId": "personal-assistant"})
	}))
	defer server.Close()
	factory, err := NewRemoteFactory([]Endpoint{endpointFor(server, "personal-assistant", "token")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Open(context.Background(), AgentSessionStart{SessionID: "session-1", AgentID: "personal-assistant", Provider: "nova"}); err == nil {
		t.Fatal("incompatible protocol accepted")
	}
}

func TestRemoteFactoryRejectsUnauthenticatedServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			_ = connection.Close()
		}
	}))
	defer server.Close()
	factory, err := NewRemoteFactory([]Endpoint{endpointFor(server, "personal-assistant", "token")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Open(context.Background(), AgentSessionStart{SessionID: "session-1", AgentID: "personal-assistant", Provider: "nova"}); err == nil {
		t.Fatal("server without HMAC proof accepted")
	}
}

func TestRemoteDisconnectBecomesRecoverableEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseHeaders := http.Header{}
		responseHeaders.Set(proofHeader, serverProof("token", "personal-assistant", r.Header.Get(nonceHeader)))
		connection, err := (&websocket.Upgrader{}).Upgrade(w, r, responseHeaders)
		if err != nil {
			return
		}
		_, _, _ = connection.ReadMessage()
		_ = connection.WriteJSON(Message{Protocol: Protocol, Kind: SessionAccepted, AgentID: "personal-assistant"})
		_ = connection.Close()
	}))
	defer server.Close()
	factory, err := NewRemoteFactory([]Endpoint{endpointFor(server, "personal-assistant", "token")})
	if err != nil {
		t.Fatal(err)
	}
	session, err := factory.Open(context.Background(), AgentSessionStart{SessionID: "session-1", AgentID: "personal-assistant", Provider: "nova"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case event := <-session.Events():
		if event.Type != AgentError || event.Code != "agent_disconnected" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect did not produce recoverable error")
	}
}

func TestRemoteFactoryRejectsProofForgedFromReceivedBearer(t *testing.T) {
	observed := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		observed <- bearer
		headers := http.Header{}
		headers.Set(proofHeader, serverProof(bearer, "personal-assistant", r.Header.Get(nonceHeader)))
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, headers)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, _, err := conn.ReadMessage(); err == nil {
			t.Error("session content sent to unauthenticated process")
		}
	}))
	defer server.Close()
	factory, err := NewRemoteFactory([]Endpoint{endpointFor(server, "personal-assistant", "secret-key")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Open(context.Background(), AgentSessionStart{SessionID: "session-1", AgentID: "personal-assistant", Provider: "nova"}); err == nil {
		t.Fatal("proof forged using received bearer accepted")
	}
	if bearer := <-observed; bearer == "secret-key" || bearer == "" {
		t.Fatal("server proof key exposed or missing client credential")
	}
}
