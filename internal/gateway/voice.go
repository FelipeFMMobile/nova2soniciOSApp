package gateway

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/session"
)

type inbound struct {
	event protocol.Event
	err   error
}
type output struct {
	event  protocol.Event
	turnID string
	done   bool
	err    error
}

func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session limit reached"})
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: s.cfg.ReadTimeout, ReadBufferSize: 4096, WriteBufferSize: 4096}
	conn, err := upgrader.Upgrade(w, r, nil) // Default origin policy requires same host, or no Origin.
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	s.connections[conn] = struct{}{}
	s.wg.Add(1)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.connections, conn); s.mu.Unlock(); s.wg.Done() }()
	s.metrics.Sessions.Add(1)
	ctx, cancel := context.WithCancel(s.ctx)
	conversation := session.New()
	in := make(chan inbound, 8)
	out := make(chan output, 8)
	readDone := make(chan struct{})
	var jobs sync.WaitGroup
	defer func() { cancel(); conversation.Close(); _ = conn.Close(); <-readDone; jobs.Wait() }()
	conn.SetReadLimit(s.cfg.MaxEventBytes)
	_ = conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(s.cfg.IdleTimeout)) })
	go func() {
		defer close(readDone)
		defer close(in)
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(s.cfg.IdleTimeout))
			event, decodeErr := protocol.DecodeClient(data)
			if kind != websocket.TextMessage {
				decodeErr = errors.New("only JSON text frames are accepted")
			}
			select {
			case in <- inbound{event, decodeErr}:
			case <-ctx.Done():
				return
			}
		}
	}()
	write := func(event protocol.Event) error {
		event.Version = protocol.Version
		event.SessionID = conversation.ID
		_ = conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
		return conn.WriteJSON(event)
	}
	sendError := func(code, message string) error {
		s.metrics.Errors.Add(1)
		return write(protocol.Event{Type: protocol.Error, TurnID: conversation.TurnID, Code: code, Message: message})
	}
	state := func() error {
		return write(protocol.Event{Type: protocol.SessionState, State: conversation.State, TurnID: conversation.TurnID})
	}
	ready := false
	startTimer := time.NewTimer(s.cfg.ReadTimeout)
	defer startTimer.Stop()
	ping := time.NewTicker(s.cfg.IdleTimeout / 3)
	defer ping.Stop()
	var turnCancel context.CancelFunc
	var startedAt time.Time
	var firstAudio time.Duration
	var outputSequence uint64
	interrupt := func(turnID string, at time.Time) error {
		if turnCancel != nil {
			turnCancel()
			turnCancel = nil
		}
		s.metrics.Interrupted.Add(1)
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
		case <-startTimer.C:
			if !ready {
				_ = sendError("session_timeout", "session.start was not received in time")
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(s.cfg.WriteTimeout)); err != nil {
				return
			}
		case input, ok := <-in:
			if !ok {
				return
			}
			if input.err != nil {
				if sendError("invalid_event", input.err.Error()) != nil {
					return
				}
				continue
			}
			event := input.event
			if !ready {
				if event.Type != protocol.SessionStart {
					if sendError("session_required", "first event must be session.start") != nil {
						return
					}
					continue
				}
				if event.Provider != "" && event.Provider != s.voice.ID() {
					if sendError("provider_unavailable", "requested provider is not available") != nil {
						return
					}
					continue
				}
				ready = true
				startTimer.Stop()
				if write(protocol.Event{Type: protocol.SessionReady, Provider: s.voice.ID(), State: session.Idle}) != nil {
					return
				}
				continue
			}
			if event.SessionID != conversation.ID {
				if sendError("session_mismatch", "sessionId does not match this connection") != nil {
					return
				}
				continue
			}
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
				if interrupted != "" {
					if interrupt(interrupted, at) != nil {
						return
					}
				}
				if oldTurn != conversation.TurnID || oldState == session.Idle {
					if write(protocol.Event{Type: protocol.TurnStarted, TurnID: conversation.TurnID}) != nil {
						return
					}
					if state() != nil {
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
				if interrupt(event.TurnID, at) != nil {
					return
				}
				if state() != nil {
					return
				}
			case protocol.SessionStop:
				conversation.Close()
				_ = write(protocol.Event{Type: protocol.SessionStopped, State: session.Closed})
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session stopped"), time.Now().Add(s.cfg.WriteTimeout))
				return
			default:
				if sendError("invalid_transition", "session is already started") != nil {
					return
				}
			}
		case item := <-out:
			// Provider output queued before cancellation must never revive stale turns.
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
					s.metrics.Completed.Add(1)
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
					s.metrics.FirstAudioCount.Add(1)
					s.metrics.FirstAudioMicros.Add(firstAudio.Microseconds())
				}
			}
			if write(event) != nil {
				return
			}
		}
	}
}
