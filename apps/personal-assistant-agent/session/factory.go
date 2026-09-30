package session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"stsmodel.local/agentwire"
	"stsmodel.local/mcpwire"
	agentconfig "stsmodel.local/personal-assistant-agent/config"
	"stsmodel.local/personal-assistant-agent/internal/actions"
	"stsmodel.local/personal-assistant-agent/internal/adapters/mcpclient"
	"stsmodel.local/personal-assistant-agent/internal/agent"
	"stsmodel.local/voicewire"
)

type Backend = orchestrator.Backend
type ToolFactory func(context.Context) (Backend, func(), error)

type Hooks struct {
	Error       func()
	Completed   func()
	Interrupted func()
	FirstAudio  func(time.Duration)
}

type Factory struct{ runtime *agent.Runtime }

func New(cfg agentconfig.Config, logger *slog.Logger, hooks Hooks, toolFactory ToolFactory) (*Factory, error) {
	if cfg.MCPBackend == "litellm" {
		if err := mcpclient.ValidateGatewayServers(cfg.LiteLLMServers); err != nil {
			return nil, err
		}
		if _, err := mcp.NewEnvelopeCodec(cfg.MCPContextSecret, cfg.LiteLLMServers[0].Alias); err != nil {
			return nil, err
		}
	}
	if len(cfg.MCPServers) > 0 {
		if err := mcpclient.ValidateServers(cfg.MCPServers); err != nil {
			return nil, err
		}
	}
	var internalFactory agent.ToolFactory
	if toolFactory != nil {
		internalFactory = func(ctx context.Context) (orchestrator.Backend, func(), error) {
			return toolFactory(ctx)
		}
	}
	runtime, err := agent.New(cfg, logger, agent.Hooks{Error: hooks.Error, Completed: hooks.Completed, Interrupted: hooks.Interrupted, FirstAudio: hooks.FirstAudio}, internalFactory)
	if err != nil {
		return nil, err
	}
	return &Factory{runtime: runtime}, nil
}

func (f *Factory) Open(ctx context.Context, start agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
	if f == nil || f.runtime == nil || start.SessionID == "" || start.AgentID != "personal-assistant" || start.Provider != "nova" {
		return nil, errors.New("invalid agent session")
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &runningSession{
		cancel: cancel,
		input:  make(chan agent.Input, 8),
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
		f.runtime.Run(sessionCtx, session.input, start.SessionID, namespace, emit)
	}()
	return session, nil
}

type runningSession struct {
	cancel    context.CancelFunc
	input     chan agent.Input
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
	case s.input <- agent.Input{Event: event}:
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
