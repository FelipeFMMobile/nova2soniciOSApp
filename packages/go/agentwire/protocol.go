package agentwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

const (
	Protocol          = "agent-session/v1alpha1"
	OpenSession       = "session.open"
	SessionAccepted   = "session.accepted"
	SessionCommand    = "session.command"
	SessionEvent      = "session.event"
	MaxMessageBytes   = 256 * 1024
	MaxAudioBytes     = 128 * 1024
	RemoteQueueDepth  = 8
	MaxRemoteSessions = 32
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type Message struct {
	Protocol string             `json:"protocol"`
	Kind     string             `json:"kind"`
	AgentID  string             `json:"agentId,omitempty"`
	Start    *AgentSessionStart `json:"start,omitempty"`
	Command  *AgentCommand      `json:"command,omitempty"`
	Event    *AgentEvent        `json:"event,omitempty"`
}

func Decode(data []byte) (Message, error) {
	if len(data) == 0 || len(data) > MaxMessageBytes {
		return Message{}, errors.New("invalid Agent Session message size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var message Message
	if err := decoder.Decode(&message); err != nil {
		return Message{}, errors.New("invalid Agent Session JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Message{}, errors.New("trailing Agent Session JSON")
	}
	if message.Protocol != Protocol {
		return Message{}, errors.New("unsupported Agent Session protocol")
	}
	return message, nil
}

func (m Message) ValidateOpen() error {
	if m.Kind != OpenSession || m.Start == nil || m.Command != nil || m.Event != nil || m.AgentID != m.Start.AgentID {
		return errors.New("invalid Agent Session open message")
	}
	if !identifier.MatchString(m.AgentID) || !identifier.MatchString(m.Start.SessionID) {
		return errors.New("invalid Agent Session identity")
	}
	if m.Start.RequestID != "" && !identifier.MatchString(m.Start.RequestID) {
		return errors.New("invalid Agent Session request identity")
	}
	return nil
}

func (m Message) ValidateAccepted(expectedAgent string) error {
	if m.Kind != SessionAccepted || m.AgentID != expectedAgent || m.Start != nil || m.Command != nil || m.Event != nil {
		return errors.New("invalid Agent Session acceptance")
	}
	return nil
}

func (m Message) ValidateCommand(sessionID string) error {
	if m.Kind != SessionCommand || m.Command == nil || m.Start != nil || m.Event != nil || m.AgentID != "" || m.Command.SessionID != sessionID {
		return errors.New("invalid Agent Session command")
	}
	c := m.Command
	switch c.Type {
	case InputAudio:
		if !identifier.MatchString(c.TurnID) || c.Sequence == 0 || len(c.Audio) == 0 || len(c.Audio) > MaxAudioBytes || len(c.Audio)%2 != 0 {
			return errors.New("invalid Agent Session audio frame")
		}
	case InputCommit, InputCancel:
		if !identifier.MatchString(c.TurnID) || c.Sequence != 0 || len(c.Audio) != 0 {
			return errors.New("invalid Agent Session turn command")
		}
	case InputConfirmation:
		if !identifier.MatchString(c.OperationID) || c.Sequence != 0 || len(c.Audio) != 0 {
			return errors.New("invalid Agent Session confirmation")
		}
	case InputClose:
		if c.TurnID != "" || c.Sequence != 0 || len(c.Audio) != 0 {
			return errors.New("invalid Agent Session close")
		}
	default:
		return fmt.Errorf("unsupported Agent Session command %q", c.Type)
	}
	return nil
}

func (m Message) ValidateEvent() error {
	if m.Kind != SessionEvent || m.Event == nil || m.Start != nil || m.Command != nil || m.AgentID != "" {
		return errors.New("invalid Agent Session event")
	}
	e := m.Event
	switch e.Type {
	case AgentSessionReady, AgentSessionRenewed:
		if e.Provider == "" || e.State == "" {
			return errors.New("invalid Agent Session lifecycle event")
		}
	case AgentSessionState, AgentSessionClosed:
		if e.State == "" {
			return errors.New("invalid Agent Session state event")
		}
	case OutputTranscript, TurnStarted, TurnCompleted, TurnInterrupted:
		if !identifier.MatchString(e.TurnID) {
			return errors.New("invalid Agent Session turn event")
		}
	case ToolStarted, ToolResult, ToolConfirmation:
		if !identifier.MatchString(e.TurnID) || e.Tool == nil || !identifier.MatchString(e.Tool.OperationID) {
			return errors.New("invalid Agent Session tool event")
		}
	case AgentError:
		if e.Code == "" || e.Message == "" {
			return errors.New("invalid Agent Session error event")
		}
	case OutputAudio:
		if !identifier.MatchString(e.TurnID) || e.Sequence == 0 || len(e.Audio) == 0 || len(e.Audio) > MaxAudioBytes || len(e.Audio)%2 != 0 || e.SampleRate <= 0 {
			return errors.New("invalid Agent Session output audio")
		}
	default:
		return fmt.Errorf("unsupported Agent Session event %q", e.Type)
	}
	return nil
}
