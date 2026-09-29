package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"stsmodel.local/voice-gateway/internal/ports"
	"stsmodel.local/voicewire"
)

type inbound struct {
	event protocol.Event
	err   error
}

type inProcessAgentFactory struct{ server *Server }

func (f inProcessAgentFactory) Open(ctx context.Context, start ports.AgentSessionStart) (ports.AgentSession, error) {
	if f.server == nil || start.SessionID == "" {
		return nil, errors.New("invalid in-process agent session")
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &inProcessAgentSession{
		cancel: cancel,
		input:  make(chan inbound, 8),
		events: make(chan ports.AgentEvent, 8),
		done:   make(chan struct{}),
	}
	go func() {
		defer close(session.done)
		defer close(session.events)
		emit := func(event protocol.Event) error {
			converted, err := agentEventFromVoice(event)
			if err != nil {
				return err
			}
			select {
			case session.events <- converted:
				return nil
			case <-sessionCtx.Done():
				return sessionCtx.Err()
			}
		}
		if f.server.cfg.Provider == "nova" {
			namespace := start.SessionID
			if start.RequestID != "" {
				namespace = start.RequestID
			}
			f.server.runNova(sessionCtx, session.input, start.SessionID, namespace, emit)
			return
		}
		f.server.runFake(sessionCtx, session.input, start.SessionID, emit)
	}()
	return session, nil
}

type inProcessAgentSession struct {
	cancel    context.CancelFunc
	input     chan inbound
	events    chan ports.AgentEvent
	done      chan struct{}
	closeOnce sync.Once
}

func (s *inProcessAgentSession) Send(ctx context.Context, command ports.AgentCommand) error {
	event, err := voiceEventFromAgentCommand(command)
	if err != nil {
		return err
	}
	select {
	case s.input <- inbound{event: event}:
		return nil
	case <-s.done:
		return errors.New("agent session closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *inProcessAgentSession) Events() <-chan ports.AgentEvent { return s.events }

func (s *inProcessAgentSession) Close() error {
	s.closeOnce.Do(s.cancel)
	<-s.done
	return nil
}

func voiceEventFromAgentCommand(command ports.AgentCommand) (protocol.Event, error) {
	event := protocol.Event{SessionID: command.SessionID, TurnID: command.TurnID, Sequence: command.Sequence}
	switch command.Type {
	case ports.InputAudio:
		if len(command.Audio) == 0 {
			return protocol.Event{}, errors.New("empty agent audio")
		}
		event.Type = protocol.AudioAppend
		event.SampleRate = protocol.InputSampleRate
		event.Audio = base64.StdEncoding.EncodeToString(command.Audio)
	case ports.InputCommit:
		event.Type = protocol.TurnCommit
	case ports.InputCancel:
		event.Type = protocol.TurnCancel
	case ports.InputConfirmation:
		event.Type = protocol.ToolConfirm
		event.Tool = &protocol.Tool{OperationID: command.OperationID, Approved: &command.Approved}
	case ports.InputClose:
		event.Type = protocol.SessionStop
	default:
		return protocol.Event{}, fmt.Errorf("unsupported agent command %q", command.Type)
	}
	return event, nil
}

func agentEventFromVoice(event protocol.Event) (ports.AgentEvent, error) {
	converted := ports.AgentEvent{
		TurnID: event.TurnID, Sequence: event.Sequence, Provider: event.Provider,
		State: event.State, SampleRate: event.SampleRate, Text: event.Text,
		Role: event.Role, Stage: event.Stage, Code: event.Code, Message: event.Message,
	}
	switch event.Type {
	case protocol.SessionReady:
		converted.Type = ports.AgentSessionReady
	case protocol.SessionState:
		converted.Type = ports.AgentSessionState
	case protocol.SessionRenewed:
		converted.Type = ports.AgentSessionRenewed
	case protocol.SessionStopped:
		converted.Type = ports.AgentSessionClosed
	case protocol.AudioOutput:
		converted.Type = ports.OutputAudio
		pcm, err := protocol.DecodeAudio(event.Audio)
		if err != nil {
			return ports.AgentEvent{}, err
		}
		converted.Audio = pcm
	case protocol.Transcript:
		converted.Type = ports.OutputTranscript
	case protocol.TurnStarted:
		converted.Type = ports.TurnStarted
	case protocol.TurnCompleted:
		converted.Type = ports.TurnCompleted
	case protocol.TurnInterrupted:
		converted.Type = ports.TurnInterrupted
	case protocol.ToolStarted:
		converted.Type = ports.ToolStarted
	case protocol.ToolResult:
		converted.Type = ports.ToolResult
	case protocol.ToolConfirmation:
		converted.Type = ports.ToolConfirmation
	case protocol.Error:
		converted.Type = ports.AgentError
	default:
		return ports.AgentEvent{}, fmt.Errorf("unsupported agent event %q", event.Type)
	}
	if event.Metrics != nil {
		converted.Metrics = &ports.AgentMetrics{FirstAudioMS: event.Metrics.FirstAudioMS, DurationMS: event.Metrics.DurationMS, InterruptionMS: event.Metrics.InterruptionMS}
	}
	if event.Tool != nil {
		converted.Tool = &ports.AgentTool{OperationID: event.Tool.OperationID, Name: event.Tool.Name, Arguments: event.Tool.Arguments, Result: event.Tool.Result}
	}
	return converted, nil
}
