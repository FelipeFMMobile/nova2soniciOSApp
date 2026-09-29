// Package agentwire defines the transport-neutral in-process AgentSession
// contract. Its types are shared by the public voice edge and agent adapters;
// business policy remains owned by the agent application.
package agentwire

import (
	"context"
	"encoding/json"
)

const (
	Idle       = "idle"
	Listening  = "listening"
	Responding = "responding"
	Closed     = "closed"
	MaxTurns   = 128

	InputAudio        = "input.audio"
	InputCommit       = "input.commit"
	InputCancel       = "input.cancel"
	InputConfirmation = "input.confirmation"
	InputClose        = "input.close"

	AgentSessionReady   = "agent.session.ready"
	AgentSessionState   = "agent.session.state"
	AgentSessionRenewed = "agent.session.renewed"
	AgentSessionClosed  = "agent.session.closed"
	OutputAudio         = "output.audio"
	OutputTranscript    = "output.transcript"
	TurnStarted         = "turn.started"
	TurnCompleted       = "turn.completed"
	TurnInterrupted     = "turn.interrupted"
	ToolStarted         = "tool.started"
	ToolResult          = "tool.result"
	ToolConfirmation    = "tool.confirmation"
	AgentError          = "agent.error"
)

type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type AgentSessionStart struct {
	SessionID string
	RequestID string
	AgentID   string
	Provider  string
}

type AgentCommand struct {
	Type        string
	SessionID   string
	TurnID      string
	Sequence    uint64
	Audio       []byte
	OperationID string
	Approved    bool
}

type AgentMetrics struct {
	FirstAudioMS   float64
	DurationMS     float64
	InterruptionMS float64
}

type AgentTool struct {
	OperationID string
	Name        string
	Arguments   json.RawMessage
	Result      json.RawMessage
}

type AgentEvent struct {
	Type       string
	TurnID     string
	Sequence   uint64
	Provider   string
	State      string
	Audio      []byte
	SampleRate int
	Text       string
	Role       string
	Stage      string
	Code       string
	Message    string
	Metrics    *AgentMetrics
	Tool       *AgentTool
}

type AgentSession interface {
	Send(context.Context, AgentCommand) error
	Events() <-chan AgentEvent
	Close() error
}

type AgentSessionFactory interface {
	Open(context.Context, AgentSessionStart) (AgentSession, error)
}
