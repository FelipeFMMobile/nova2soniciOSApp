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
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Principal is the authenticated client identity asserted by the voice edge.
// Subject is opaque to the transport and must never be accepted from Voice v1.
type Principal struct {
	Subject string `json:"subject"`
}

type AgentSessionStart struct {
	SessionID string    `json:"sessionId"`
	RequestID string    `json:"requestId,omitempty"`
	AgentID   string    `json:"agentId"`
	Provider  string    `json:"provider,omitempty"`
	Principal Principal `json:"principal"`
}

type AgentCommand struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	TurnID      string `json:"turnId,omitempty"`
	Sequence    uint64 `json:"sequence,omitempty"`
	Audio       []byte `json:"audio,omitempty"`
	OperationID string `json:"operationId,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
}

type AgentMetrics struct {
	FirstAudioMS   float64 `json:"firstAudioMs,omitempty"`
	DurationMS     float64 `json:"durationMs,omitempty"`
	InterruptionMS float64 `json:"interruptionMs,omitempty"`
}

type AgentTool struct {
	OperationID string          `json:"operationId"`
	Name        string          `json:"name,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

type AgentEvent struct {
	Type       string        `json:"type"`
	TurnID     string        `json:"turnId,omitempty"`
	Sequence   uint64        `json:"sequence,omitempty"`
	Provider   string        `json:"provider,omitempty"`
	State      string        `json:"state,omitempty"`
	Audio      []byte        `json:"audio,omitempty"`
	SampleRate int           `json:"sampleRate,omitempty"`
	Text       string        `json:"text,omitempty"`
	Role       string        `json:"role,omitempty"`
	Stage      string        `json:"stage,omitempty"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
	Metrics    *AgentMetrics `json:"metrics,omitempty"`
	Tool       *AgentTool    `json:"tool,omitempty"`
}

type AgentSession interface {
	Send(context.Context, AgentCommand) error
	Events() <-chan AgentEvent
	Close() error
}

type AgentSessionFactory interface {
	Open(context.Context, AgentSessionStart) (AgentSession, error)
}
