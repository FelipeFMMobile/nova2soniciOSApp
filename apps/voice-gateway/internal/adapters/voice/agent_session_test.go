package gateway

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"stsmodel.local/agentwire"
	"stsmodel.local/voicewire"
)

type recordingAgentFactory struct {
	starts   chan agentwire.AgentSessionStart
	sessions chan *recordingAgentSession
}

func TestVoiceHandlerUsesRemoteAgentSession(t *testing.T) {
	remoteFactory := &recordingAgentFactory{starts: make(chan agentwire.AgentSessionStart, 1), sessions: make(chan *recordingAgentSession, 1)}
	handler, err := agentwire.NewHandler(agentwire.ServerConfig{AgentID: "personal-assistant", Token: "internal-token", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: 3 * time.Second, MaxSessions: 2}, remoteFactory)
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(handler)
	defer remote.Close()
	cfg := testConfig()
	cfg.AgentMode = "remote"
	cfg.AgentPromptFile = ""
	cfg.MCPBackend = ""
	cfg.DefaultAgentID = "personal-assistant"
	cfg.AgentCatalog = []agentwire.Endpoint{{AgentID: "personal-assistant", URL: "ws" + strings.TrimPrefix(remote.URL, "http") + "/v1alpha1/session", Token: "internal-token"}}
	_, _, url := setup(t, cfg)
	c := dial(t, url, "")
	send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "fake", RequestID: "remote-contract"})
	ready := until(t, c, protocol.SessionReady)
	start := <-remoteFactory.starts
	agent := <-remoteFactory.sessions
	if start.SessionID != ready.SessionID || start.RequestID != "remote-contract" || start.AgentID != "personal-assistant" {
		t.Fatal(start, ready)
	}
	pcm := []byte{0, 0, 1, 0}
	send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: ready.SessionID, TurnID: "turn-remote", Sequence: 1, SampleRate: 16000, Audio: base64.StdEncoding.EncodeToString(pcm)})
	if command := <-agent.commands; command.Type != agentwire.InputAudio || command.TurnID != "turn-remote" || string(command.Audio) != string(pcm) {
		t.Fatal(command)
	}
	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: ready.SessionID})
	if command := <-agent.commands; command.Type != agentwire.InputClose {
		t.Fatal(command)
	}
	until(t, c, protocol.SessionStopped)
}

func (f *recordingAgentFactory) Open(_ context.Context, start agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
	session := &recordingAgentSession{events: make(chan agentwire.AgentEvent, 16), commands: make(chan agentwire.AgentCommand, 16)}
	f.starts <- start
	f.sessions <- session
	session.events <- agentwire.AgentEvent{Type: agentwire.AgentSessionReady, Provider: "fake", State: "idle"}
	return session, nil
}

type recordingAgentSession struct {
	events    chan agentwire.AgentEvent
	commands  chan agentwire.AgentCommand
	closeOnce sync.Once
}

func (s *recordingAgentSession) Send(_ context.Context, command agentwire.AgentCommand) error {
	s.commands <- command
	switch command.Type {
	case agentwire.InputAudio:
		s.events <- agentwire.AgentEvent{Type: agentwire.TurnStarted, TurnID: command.TurnID}
		s.events <- agentwire.AgentEvent{Type: agentwire.OutputAudio, TurnID: command.TurnID, Sequence: 1, SampleRate: 24000, Audio: []byte{0, 0}}
	case agentwire.InputClose:
		s.events <- agentwire.AgentEvent{Type: agentwire.AgentSessionClosed, State: "closed"}
	}
	return nil
}

func (s *recordingAgentSession) Events() <-chan agentwire.AgentEvent { return s.events }
func (s *recordingAgentSession) Close() error {
	s.closeOnce.Do(func() { close(s.events) })
	return nil
}

func TestVoiceHandlerUsesAgentSessionPort(t *testing.T) {
	s, _, url := setup(t, testConfig())
	factory := &recordingAgentFactory{starts: make(chan agentwire.AgentSessionStart, 1), sessions: make(chan *recordingAgentSession, 1)}
	s.agentFactory = factory
	c := dial(t, url, "")
	send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "fake", RequestID: "port-contract"})
	ready := until(t, c, protocol.SessionReady)
	start := <-factory.starts
	agent := <-factory.sessions
	if start.SessionID != ready.SessionID || start.RequestID != "port-contract" || start.AgentID != "personal-assistant" || start.Provider != "fake" {
		t.Fatal("session start was not translated", start, ready)
	}
	appendAudio(t, c, ready.SessionID, "turn-1", 1)
	audio := until(t, c, protocol.AudioOutput)
	if audio.TurnID != "turn-1" || audio.SampleRate != 24000 {
		t.Fatal("agent output was not translated", audio)
	}
	pcm, err := base64.StdEncoding.DecodeString(audio.Audio)
	if err != nil || len(pcm) != 2 {
		t.Fatal("agent PCM was not encoded for Voice v1", err, pcm)
	}
	command := <-agent.commands
	if command.Type != agentwire.InputAudio || command.SessionID != ready.SessionID || command.TurnID != "turn-1" || command.Sequence != 1 || len(command.Audio) != 4 {
		t.Fatal("Voice v1 audio was not decoded for the agent", command)
	}

	agent.events <- agentwire.AgentEvent{Type: agentwire.ToolConfirmation, TurnID: "turn-1", Tool: &agentwire.AgentTool{OperationID: "operation-1", Name: "notes.delete"}}
	until(t, c, protocol.ToolConfirmation)
	approved := true
	send(t, c, protocol.Event{Type: protocol.ToolConfirm, SessionID: ready.SessionID, Tool: &protocol.Tool{OperationID: "operation-1", Approved: &approved}})
	command = <-agent.commands
	if command.Type != agentwire.InputConfirmation || command.OperationID != "operation-1" || !command.Approved {
		t.Fatal("UI confirmation was not translated", command)
	}

	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: ready.SessionID})
	until(t, c, protocol.SessionStopped)
	if command = <-agent.commands; command.Type != agentwire.InputClose {
		t.Fatal("session close was not translated", command)
	}
}
