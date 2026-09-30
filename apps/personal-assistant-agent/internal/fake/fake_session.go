package fake

import (
	"context"
	"errors"
	"sync"
	"time"

	"stsmodel.local/personal-assistant-agent/internal/fake/conversation"
	"stsmodel.local/voicewire"
)

type output struct {
	event  protocol.Event
	turnID string
	done   bool
	err    error
}

func (s *Server) runFake(ctx context.Context, in <-chan inbound, sessionID string, write func(protocol.Event) error) {
	conversation := session.New()
	conversation.ID = sessionID
	out := make(chan output, 8)
	var jobs sync.WaitGroup
	defer func() { conversation.Close(); jobs.Wait() }()
	sendError := func(code, message string) error {

		return write(protocol.Event{Type: protocol.Error, TurnID: conversation.TurnID, Code: code, Message: message})
	}
	state := func() error {
		return write(protocol.Event{Type: protocol.SessionState, State: conversation.State, TurnID: conversation.TurnID})
	}
	if write(protocol.Event{Type: protocol.SessionReady, Provider: s.voice.ID(), State: session.Idle}) != nil {
		return
	}
	var turnCancel context.CancelFunc
	var startedAt time.Time
	var firstAudio time.Duration
	var outputSequence uint64
	interrupt := func(turnID string, at time.Time) error {
		if turnCancel != nil {
			turnCancel()
			turnCancel = nil
		}

		return write(protocol.Event{Type: protocol.TurnInterrupted, TurnID: turnID, Metrics: &protocol.Metrics{InterruptionMS: float64(time.Since(at).Microseconds()) / 1000}})
	}
	startProvider := func(pcm []byte) {
		turnID := conversation.TurnID
		turnCtx, stop := context.WithTimeout(ctx, s.cfg.TurnTimeout)
		turnCancel = stop
		startedAt = time.Now()
		firstAudio = 0
		outputSequence = 0
		jobs.Add(1)
		go func() {
			defer jobs.Done()
			defer stop()
			err := s.voice.Respond(turnCtx, pcm, func(event protocol.Event) error {
				select {
				case <-turnCtx.Done():
					return turnCtx.Err()
				case out <- output{event: event, turnID: turnID}:
					return nil
				}
			})
			select {
			case out <- output{turnID: turnID, done: true, err: err}:
			case <-ctx.Done():
			}
		}()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case input, ok := <-in:
			if !ok {
				return
			}
			event := input.event
			switch event.Type {
			case protocol.AudioAppend:
				at := time.Now()
				oldTurn, oldState := conversation.TurnID, conversation.State
				interrupted, err := conversation.Append(event)
				if err != nil {
					if sendError("invalid_audio", err.Error()) != nil {
						return
					}
					continue
				}
				if interrupted != "" && interrupt(interrupted, at) != nil {
					return
				}
				if oldTurn != conversation.TurnID || oldState == session.Idle {
					if write(protocol.Event{Type: protocol.TurnStarted, TurnID: conversation.TurnID}) != nil || state() != nil {
						return
					}
				}
			case protocol.TurnCommit:
				pcm, err := conversation.Commit(event.TurnID)
				if err != nil {
					if sendError("invalid_turn", err.Error()) != nil {
						return
					}
					continue
				}
				if state() != nil {
					return
				}
				startProvider(pcm)
			case protocol.TurnCancel:
				at := time.Now()
				if err := conversation.Cancel(event.TurnID); err != nil {
					if sendError("invalid_turn", err.Error()) != nil {
						return
					}
					continue
				}
				if interrupt(event.TurnID, at) != nil || state() != nil {
					return
				}
			case protocol.SessionStop:
				conversation.Close()
				_ = write(protocol.Event{Type: protocol.SessionStopped, State: session.Closed})
				return
			default:
				if sendError("invalid_transition", "session is already started") != nil {
					return
				}
			}
		case item := <-out:
			if conversation.State != session.Responding || item.turnID != conversation.TurnID {
				continue
			}
			if item.done {
				conversation.Complete(item.turnID)
				turnCancel = nil
				if item.err != nil {
					code := "provider_failed"
					if errors.Is(item.err, context.DeadlineExceeded) {
						code = "provider_timeout"
					}
					if sendError(code, "provider could not complete the turn") != nil {
						return
					}
				} else {

					metrics := &protocol.Metrics{FirstAudioMS: float64(firstAudio.Microseconds()) / 1000, DurationMS: float64(time.Since(startedAt).Microseconds()) / 1000}
					if write(protocol.Event{Type: protocol.TurnCompleted, TurnID: item.turnID, Metrics: metrics}) != nil {
						return
					}
					s.logger.Info("voice turn completed", "session_id", conversation.ID, "turn_id", item.turnID, "first_audio_ms", metrics.FirstAudioMS, "duration_ms", metrics.DurationMS)
				}
				if state() != nil {
					return
				}
				continue
			}
			event := item.event
			event.TurnID = item.turnID
			if event.Type == protocol.AudioOutput {
				outputSequence++
				event.Sequence = outputSequence
				if firstAudio == 0 {
					firstAudio = time.Since(startedAt)

				}
			}
			if write(event) != nil {
				return
			}
		}
	}
}
