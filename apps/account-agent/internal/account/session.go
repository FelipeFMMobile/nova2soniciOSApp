package account

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"stsmodel.local/agentwire"

	"stsmodel.local/actionruntime"

	"stsmodel.local/voicewire"
)

type Factory struct {
	Config Config
	Logger *slog.Logger
}

func (f *Factory) Open(ctx context.Context, start agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
	if f == nil || start.AgentID != "account" || start.Protocol != agentwire.ProtocolV2 || start.Principal.DeviceID == "" {
		return nil, errors.New("invalid account session")
	}
	backend, err := NewMCP(ctx, f.Config, start.Principal)
	if err != nil {
		return nil, err
	}
	if err := backend.VerifyGuardrails(ctx); err != nil {
		return nil, err
	}
	if f.Config.Provider == "fake" {
		return newFake(ctx, start, backend), nil
	}
	runtime, err := New(f.Config, f.Logger, Hooks{}, func(context.Context) (orchestrator.Backend, func(), error) { return backend, func() {}, nil })
	if err != nil {
		return nil, err
	}
	runtime.authenticated = start.Principal.Authenticated
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &runningSession{
		cancel: cancel,
		input:  make(chan Input, 8),
		events: make(chan agentwire.AgentEvent, 8),
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
		namespace := start.SessionID
		if start.RequestID != "" {
			namespace = start.RequestID
		}
		runtime.Run(sessionCtx, session.input, start.SessionID, namespace, emit)
	}()
	return session, nil
}

type runningSession struct {
	cancel    context.CancelFunc
	input     chan Input
	events    chan agentwire.AgentEvent
	done      chan struct{}
	closeOnce sync.Once
}

func (s *runningSession) Send(ctx context.Context, command agentwire.AgentCommand) error {
	event, err := voiceEventFromAgentCommand(command)
	if err != nil {
		return err
	}
	select {
	case s.input <- Input{Event: event}:
		return nil
	case <-s.done:
		return errors.New("agent session closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *runningSession) Events() <-chan agentwire.AgentEvent { return s.events }

func (s *runningSession) Close() error {
	s.closeOnce.Do(s.cancel)
	<-s.done
	return nil
}

func voiceEventFromAgentCommand(command agentwire.AgentCommand) (protocol.Event, error) {
	event := protocol.Event{SessionID: command.SessionID, TurnID: command.TurnID, Sequence: command.Sequence}
	switch command.Type {
	case agentwire.InputAudio:
		if len(command.Audio) == 0 {
			return protocol.Event{}, errors.New("empty agent audio")
		}
		event.Type = protocol.AudioAppend
		event.SampleRate = protocol.InputSampleRate
		event.Audio = base64.StdEncoding.EncodeToString(command.Audio)
	case agentwire.InputCommit:
		event.Type = protocol.TurnCommit
	case agentwire.InputCancel:
		event.Type = protocol.TurnCancel
	case agentwire.InputConfirmation:
		event.Type = protocol.ToolConfirm
		event.Tool = &protocol.Tool{OperationID: command.OperationID, Approved: &command.Approved}
	case agentwire.InputClose:
		event.Type = protocol.SessionStop
	default:
		return protocol.Event{}, fmt.Errorf("unsupported agent command %q", command.Type)
	}
	return event, nil
}

func agentEventFromVoice(event protocol.Event) (agentwire.AgentEvent, error) {
	converted := agentwire.AgentEvent{
		TurnID: event.TurnID, Sequence: event.Sequence, Provider: event.Provider,
		State: event.State, SampleRate: event.SampleRate, Text: event.Text,
		Role: event.Role, Stage: event.Stage, Code: event.Code, Message: event.Message,
	}
	switch event.Type {
	case "ui.a2ui":
		converted.Type = "ui.a2ui"
		converted.UI = event.UI
	case protocol.SessionReady:
		converted.Type = agentwire.AgentSessionReady
	case protocol.SessionState:
		converted.Type = agentwire.AgentSessionState
	case protocol.SessionRenewed:
		converted.Type = agentwire.AgentSessionRenewed
	case protocol.SessionStopped:
		converted.Type = agentwire.AgentSessionClosed
	case protocol.AudioOutput:
		converted.Type = agentwire.OutputAudio
		pcm, err := protocol.DecodeAudio(event.Audio)
		if err != nil {
			return agentwire.AgentEvent{}, err
		}
		converted.Audio = pcm
	case protocol.Transcript:
		converted.Type = agentwire.OutputTranscript
	case protocol.TurnStarted:
		converted.Type = agentwire.TurnStarted
	case protocol.TurnCompleted:
		converted.Type = agentwire.TurnCompleted
	case protocol.TurnInterrupted:
		converted.Type = agentwire.TurnInterrupted
	case protocol.ToolStarted:
		converted.Type = agentwire.ToolStarted
	case protocol.ToolResult:
		converted.Type = agentwire.ToolResult
	case protocol.ToolConfirmation:
		converted.Type = agentwire.ToolConfirmation
	case protocol.Error:
		converted.Type = agentwire.AgentError
	default:
		return agentwire.AgentEvent{}, fmt.Errorf("unsupported agent event %q", event.Type)
	}
	if event.Metrics != nil {
		converted.Metrics = &agentwire.AgentMetrics{FirstAudioMS: event.Metrics.FirstAudioMS, DurationMS: event.Metrics.DurationMS, InterruptionMS: event.Metrics.InterruptionMS}
	}
	if event.Tool != nil {
		converted.Tool = &agentwire.AgentTool{OperationID: event.Tool.OperationID, Name: event.Tool.Name, Arguments: event.Tool.Arguments, Result: event.Tool.Result}
	}
	return converted, nil
}
