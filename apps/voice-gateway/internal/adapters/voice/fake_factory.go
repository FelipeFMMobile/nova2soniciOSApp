package gateway

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"stsmodel.local/agentwire"
	"stsmodel.local/voicewire"
)

// inProcessFakeFactory is a deterministic transport fixture. The real
// personal assistant implementation lives in its own application module.
type inProcessFakeFactory struct{ server *Server }

type inbound struct {
	event protocol.Event
	err   error
}

func (f inProcessFakeFactory) Open(ctx context.Context, start agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
	if f.server == nil || start.SessionID == "" || start.Provider != "fake" {
		return nil, errors.New("invalid fake agent session")
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	s := &fakeAgentSession{cancel: cancel, input: make(chan inbound, 8), events: make(chan agentwire.AgentEvent, 8), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer close(s.events)
		f.server.runFake(sessionCtx, s.input, start.SessionID, func(event protocol.Event) error {
			converted, err := fakeAgentEvent(event)
			if err != nil {
				return err
			}
			select {
			case s.events <- converted:
				return nil
			case <-sessionCtx.Done():
				return sessionCtx.Err()
			}
		})
	}()
	return s, nil
}

type fakeAgentSession struct {
	cancel context.CancelFunc
	input  chan inbound
	events chan agentwire.AgentEvent
	done   chan struct{}
	once   sync.Once
}

func (s *fakeAgentSession) Send(ctx context.Context, command agentwire.AgentCommand) error {
	event, err := fakeVoiceCommand(command)
	if err != nil {
		return err
	}
	select {
	case s.input <- inbound{event: event}:
		return nil
	case <-s.done:
		return errors.New("fake agent session closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *fakeAgentSession) Events() <-chan agentwire.AgentEvent { return s.events }

func (s *fakeAgentSession) Close() error {
	s.once.Do(s.cancel)
	<-s.done
	return nil
}

func fakeVoiceCommand(command agentwire.AgentCommand) (protocol.Event, error) {
	event := protocol.Event{SessionID: command.SessionID, TurnID: command.TurnID, Sequence: command.Sequence}
	switch command.Type {
	case agentwire.InputAudio:
		if len(command.Audio) == 0 {
			return protocol.Event{}, errors.New("empty fake audio")
		}
		event.Type = protocol.AudioAppend
		event.SampleRate = protocol.InputSampleRate
		event.Audio = base64.StdEncoding.EncodeToString(command.Audio)
	case agentwire.InputCommit:
		event.Type = protocol.TurnCommit
	case agentwire.InputCancel:
		event.Type = protocol.TurnCancel
	case agentwire.InputClose:
		event.Type = protocol.SessionStop
	case agentwire.InputConfirmation:
		return protocol.Event{}, errors.New("fake agent has no tools")
	default:
		return protocol.Event{}, fmt.Errorf("unsupported fake agent command %q", command.Type)
	}
	return event, nil
}

func fakeAgentEvent(event protocol.Event) (agentwire.AgentEvent, error) {
	converted := agentwire.AgentEvent{TurnID: event.TurnID, Sequence: event.Sequence, Provider: event.Provider, State: event.State, SampleRate: event.SampleRate, Text: event.Text, Role: event.Role, Stage: event.Stage, Code: event.Code, Message: event.Message}
	switch event.Type {
	case protocol.SessionReady:
		converted.Type = agentwire.AgentSessionReady
	case protocol.SessionState:
		converted.Type = agentwire.AgentSessionState
	case protocol.SessionStopped:
		converted.Type = agentwire.AgentSessionClosed
	case protocol.AudioOutput:
		pcm, err := protocol.DecodeAudio(event.Audio)
		if err != nil {
			return agentwire.AgentEvent{}, err
		}
		converted.Type, converted.Audio = agentwire.OutputAudio, pcm
	case protocol.Transcript:
		converted.Type = agentwire.OutputTranscript
	case protocol.TurnStarted:
		converted.Type = agentwire.TurnStarted
	case protocol.TurnCompleted:
		converted.Type = agentwire.TurnCompleted
	case protocol.TurnInterrupted:
		converted.Type = agentwire.TurnInterrupted
	case protocol.Error:
		converted.Type = agentwire.AgentError
	default:
		return agentwire.AgentEvent{}, fmt.Errorf("unsupported fake agent event %q", event.Type)
	}
	if event.Metrics != nil {
		converted.Metrics = &agentwire.AgentMetrics{FirstAudioMS: event.Metrics.FirstAudioMS, DurationMS: event.Metrics.DurationMS, InterruptionMS: event.Metrics.InterruptionMS}
	}
	return converted, nil
}
