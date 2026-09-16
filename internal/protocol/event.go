// Package protocol defines the public, versioned Apple/terminal voice contract.
package protocol

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

const Version = 1
const InputSampleRate = 16000
const MaxAudioChunkBytes = 32000 // One second of mono PCM16 input.

const (
	SessionStart     = "session.start"
	SessionReady     = "session.ready"
	SessionState     = "session.state"
	SessionStop      = "session.stop"
	SessionStopped   = "session.stopped"
	SessionRenewed   = "session.renewed"
	AudioAppend      = "audio.append"
	TurnCommit       = "turn.commit"
	TurnCancel       = "turn.cancel"
	TurnStarted      = "turn.started"
	TurnCompleted    = "turn.completed"
	TurnInterrupted  = "turn.interrupted"
	Transcript       = "transcript"
	AudioOutput      = "audio.output"
	ToolStarted      = "tool.started"
	ToolResult       = "tool.result"
	ToolConfirmation = "tool.confirmation"
	Error            = "error"
)

type Metrics struct {
	FirstAudioMS   float64 `json:"firstAudioMs"`
	DurationMS     float64 `json:"durationMs"`
	InterruptionMS float64 `json:"interruptionMs,omitempty"`
}

// Tool events are reserved for Stage 6; clients must not execute tools locally.
type Tool struct {
	OperationID string          `json:"operationId"`
	Name        string          `json:"name"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

type Event struct {
	Version    int      `json:"version"`
	Type       string   `json:"type"`
	SessionID  string   `json:"sessionId,omitempty"`
	TurnID     string   `json:"turnId,omitempty"`
	Sequence   uint64   `json:"sequence,omitempty"`
	Provider   string   `json:"provider,omitempty"`
	State      string   `json:"state,omitempty"`
	Audio      string   `json:"audio,omitempty"`
	SampleRate int      `json:"sampleRate,omitempty"`
	Text       string   `json:"text,omitempty"`
	Role       string   `json:"role,omitempty"`
	Stage      string   `json:"stage,omitempty"`
	Code       string   `json:"code,omitempty"`
	Message    string   `json:"message,omitempty"`
	Metrics    *Metrics `json:"metrics,omitempty"`
	Tool       *Tool    `json:"tool,omitempty"`
}

func DecodeClient(data []byte) (Event, error) {
	var event Event
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("invalid JSON event")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Event{}, fmt.Errorf("expected one JSON event")
	}
	if event.Version != Version {
		return Event{}, fmt.Errorf("unsupported protocol version")
	}
	if (event.SessionID != "" && !ValidID(event.SessionID)) || (event.TurnID != "" && !ValidID(event.TurnID)) {
		return Event{}, fmt.Errorf("identifiers must be 1-128 ASCII letters, digits, underscores or hyphens")
	}
	switch event.Type {
	case SessionStart:
		if event.SessionID != "" || event.TurnID != "" || event.Sequence != 0 {
			return Event{}, fmt.Errorf("session.start cannot reuse identifiers")
		}
	case AudioAppend:
		if event.TurnID == "" || event.Sequence == 0 || event.SampleRate != InputSampleRate {
			return Event{}, fmt.Errorf("audio requires turnId, sequence and sampleRate 16000")
		}
		if _, err := DecodeAudio(event.Audio); err != nil {
			return Event{}, err
		}
	case TurnCommit, TurnCancel:
		if event.TurnID == "" {
			return Event{}, fmt.Errorf("turnId is required")
		}
	case SessionStop:
	default:
		return Event{}, fmt.Errorf("unsupported client event")
	}
	if event.Type != SessionStart && event.SessionID == "" {
		return Event{}, fmt.Errorf("sessionId is required")
	}
	if event.Text != "" || event.State != "" || event.Role != "" || event.Stage != "" || event.Code != "" || event.Message != "" || event.Metrics != nil || event.Tool != nil {
		return Event{}, fmt.Errorf("server fields are not allowed in client events")
	}
	if event.Type != AudioAppend && (event.Audio != "" || event.SampleRate != 0) {
		return Event{}, fmt.Errorf("audio fields require audio.append")
	}
	if event.Type != SessionStart && event.Provider != "" {
		return Event{}, fmt.Errorf("provider is only allowed in session.start")
	}
	if event.Type != AudioAppend && event.Sequence != 0 {
		return Event{}, fmt.Errorf("sequence is only allowed in audio.append")
	}
	return event, nil
}

func ValidID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func DecodeAudio(encoded string) ([]byte, error) {
	if len(encoded) > base64.StdEncoding.EncodedLen(MaxAudioChunkBytes) {
		return nil, fmt.Errorf("audio chunk exceeds one second")
	}
	pcm, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(pcm) == 0 || len(pcm)%2 != 0 {
		return nil, fmt.Errorf("audio must be nonempty base64 PCM16")
	}
	return pcm, nil
}
