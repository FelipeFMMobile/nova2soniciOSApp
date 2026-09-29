package gateway

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"

	"stsmodel.local/voice-gateway/internal/ports"
	"stsmodel.local/voicewire"
)

type recordingAgentFactory struct {
	starts   chan ports.AgentSessionStart
	sessions chan *recordingAgentSession
}

func (f *recordingAgentFactory) Open(_ context.Context, start ports.AgentSessionStart) (ports.AgentSession, error) {
	session := &recordingAgentSession{events: make(chan ports.AgentEvent, 16), commands: make(chan ports.AgentCommand, 16)}
	f.starts <- start
	f.sessions <- session
	session.events <- ports.AgentEvent{Type: ports.AgentSessionReady, Provider: "fake", State: "idle"}
	return session, nil
}

type recordingAgentSession struct {
	events    chan ports.AgentEvent
	commands  chan ports.AgentCommand
	closeOnce sync.Once
}

func (s *recordingAgentSession) Send(_ context.Context, command ports.AgentCommand) error {
	s.commands <- command
	switch command.Type {
	case ports.InputAudio:
		s.events <- ports.AgentEvent{Type: ports.TurnStarted, TurnID: command.TurnID}
		s.events <- ports.AgentEvent{Type: ports.OutputAudio, TurnID: command.TurnID, Sequence: 1, SampleRate: 24000, Audio: []byte{0, 0}}
	case ports.InputClose:
		s.events <- ports.AgentEvent{Type: ports.AgentSessionClosed, State: "closed"}
	}
	return nil
}

func (s *recordingAgentSession) Events() <-chan ports.AgentEvent { return s.events }
func (s *recordingAgentSession) Close() error {
	s.closeOnce.Do(func() { close(s.events) })
	return nil
}

func TestVoiceHandlerUsesAgentSessionPort(t *testing.T) {
	s, _, url := setup(t, testConfig())
	factory := &recordingAgentFactory{starts: make(chan ports.AgentSessionStart, 1), sessions: make(chan *recordingAgentSession, 1)}
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
	if command.Type != ports.InputAudio || command.SessionID != ready.SessionID || command.TurnID != "turn-1" || command.Sequence != 1 || len(command.Audio) != 4 {
		t.Fatal("Voice v1 audio was not decoded for the agent", command)
	}

	agent.events <- ports.AgentEvent{Type: ports.ToolConfirmation, TurnID: "turn-1", Tool: &ports.AgentTool{OperationID: "operation-1", Name: "notes.delete"}}
	until(t, c, protocol.ToolConfirmation)
	approved := true
	send(t, c, protocol.Event{Type: protocol.ToolConfirm, SessionID: ready.SessionID, Tool: &protocol.Tool{OperationID: "operation-1", Approved: &approved}})
	command = <-agent.commands
	if command.Type != ports.InputConfirmation || command.OperationID != "operation-1" || !command.Approved {
		t.Fatal("UI confirmation was not translated", command)
	}

	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: ready.SessionID})
	until(t, c, protocol.SessionStopped)
	if command = <-agent.commands; command.Type != ports.InputClose {
		t.Fatal("session close was not translated", command)
	}
}
