package gateway

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/provider/nova"
	"stsmodel.local/poc/internal/session"
)

// runNova owns all Nova state and writes. Microphone input remains live while
// inference responds; only the private stream reader runs asynchronously.
func (s *Server) runNova(ctx context.Context, conn *websocket.Conn, in <-chan inbound, sessionID string, write func(protocol.Event) error) {
	sendError := func(code, message string) {
		s.metrics.Errors.Add(1)
		_ = write(protocol.Event{Type: protocol.Error, Code: code, Message: message})
	}
	startDeadline := time.NewTimer(s.cfg.ReadTimeout)
	defer startDeadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-startDeadline.C:
			sendError("session_timeout", "session.start was not received in time")
			return
		case input, ok := <-in:
			if !ok {
				return
			}
			if input.err != nil {
				sendError("invalid_event", input.err.Error())
				continue
			}
			if input.event.Type != protocol.SessionStart {
				sendError("session_required", "first event must be session.start")
				continue
			}
			if input.event.Provider != "" && input.event.Provider != "nova" {
				sendError("provider_unavailable", "requested provider is not selected")
				continue
			}
			goto started
		}
	}
started:
	startDeadline.Stop()
	stream, err := nova.Open(ctx, s.cfg.NovaBridgeURL, nil)
	if err != nil {
		sendError("provider_unavailable", err.Error())
		return
	}
	defer func() { stream.Close() }()
	mapper := nova.NewMapper(nil)
	if write(protocol.Event{Type: protocol.SessionReady, Provider: "nova", State: session.Idle}) != nil {
		return
	}
	inputID := ""
	inputSequence := uint64(0)
	usedInputs := map[string]bool{}
	openedAt := time.Now()
	renew := time.NewTimer(s.cfg.NovaMaxSessionAge)
	defer renew.Stop()
	ping := time.NewTicker(s.cfg.IdleTimeout / 3)
	defer ping.Stop()
	turnTimer := time.NewTimer(time.Hour)
	turnTimer.Stop()
	defer turnTimer.Stop()
	var turnTimeout <-chan time.Time
	emit := func(events []protocol.Event) bool {
		for _, event := range events {
			if event.Type == protocol.SessionState && event.State == session.Responding && turnTimeout == nil {
				turnTimer.Reset(s.cfg.TurnTimeout)
				turnTimeout = turnTimer.C
			}
			if event.Type == protocol.AudioOutput && event.Sequence == 1 {
				s.metrics.FirstAudioCount.Add(1)
				s.metrics.FirstAudioMicros.Add(mapper.FirstAudioMicros())
			}
			if event.Type == protocol.TurnInterrupted {
				s.metrics.Interrupted.Add(1)
				turnTimer.Stop()
				turnTimeout = nil
			}
			if event.Type == protocol.TurnCompleted {
				s.metrics.Completed.Add(1)
				turnTimer.Stop()
				turnTimeout = nil
				s.logger.Info("nova turn completed", "turn_id", event.TurnID, "first_audio_ms", event.Metrics.FirstAudioMS, "duration_ms", event.Metrics.DurationMS)
			}
			if write(event) != nil {
				return false
			}
		}
		return true
	}
	rotate := func() bool {
		history := mapper.History
		stream.Close()
		replacement, err := nova.Open(ctx, s.cfg.NovaBridgeURL, history)
		if err != nil {
			sendError("provider_unavailable", "Nova session renewal failed")
			return false
		}
		stream = replacement
		mapper = nova.NewMapper(history)
		openedAt = time.Now()
		renew.Reset(s.cfg.NovaMaxSessionAge)
		return write(protocol.Event{Type: protocol.SessionRenewed, Provider: "nova", State: session.Idle}) == nil
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(s.cfg.WriteTimeout)) != nil {
				return
			}
		case <-turnTimeout:
			if !emit(mapper.Interrupt()) {
				return
			}
			sendError("provider_timeout", "Nova did not complete the turn in time")
			return
		case <-renew.C:
			if mapper.State != session.Idle && time.Since(openedAt) < 7*time.Minute+50*time.Second {
				renew.Reset(10 * time.Second)
				continue
			}
			if mapper.State != session.Idle && !emit(mapper.Interrupt()) {
				return
			}
			if !rotate() {
				return
			}
		case input, ok := <-in:
			if !ok {
				return
			}
			if input.err != nil {
				sendError("invalid_event", input.err.Error())
				continue
			}
			e := input.event
			if e.SessionID != sessionID {
				sendError("session_mismatch", "sessionId does not match this connection")
				continue
			}
			switch e.Type {
			case protocol.AudioAppend:
				if e.TurnID != inputID {
					if e.Sequence != 1 || usedInputs[e.TurnID] || len(usedInputs) >= session.MaxTurns {
						sendError("invalid_audio", "new input requires an unused turnId and sequence 1")
						continue
					}
					inputID = e.TurnID
					inputSequence = 0
					usedInputs[inputID] = true
					if !emit(mapper.StartInput(inputID)) {
						return
					}
				}
				if e.Sequence != inputSequence+1 {
					sendError("invalid_audio", "unexpected input sequence")
					continue
				}
				inputSequence = e.Sequence
				if stream.SendAudio(e.Audio) != nil {
					sendError("provider_unavailable", "Nova audio transport disconnected")
					return
				}
			case protocol.TurnCommit:
				if e.TurnID != inputID || inputSequence == 0 {
					sendError("invalid_turn", "no matching input to commit")
					continue
				}
				mapper.Commit()
				turnTimer.Reset(s.cfg.TurnTimeout)
				turnTimeout = turnTimer.C
			case protocol.TurnCancel:
				if e.TurnID != mapper.TurnID || mapper.State == session.Idle {
					sendError("invalid_turn", "no matching active turn")
					continue
				}
				if !emit(mapper.Interrupt()) {
					return
				}
				turnTimer.Stop()
				turnTimeout = nil
				inputID = ""
				inputSequence = 0
				if !rotate() {
					return
				}
			case protocol.SessionStop:
				_ = write(protocol.Event{Type: protocol.SessionStopped, State: session.Closed})
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session stopped"), time.Now().Add(s.cfg.WriteTimeout))
				return
			default:
				sendError("invalid_transition", "session is already started")
			}
		case event, ok := <-stream.Events():
			if !ok {
				sendError("provider_unavailable", "Nova stream disconnected")
				return
			}
			events, err := mapper.Map(event)
			if err != nil {
				sendError("provider_failed", "Nova returned an invalid response")
				return
			}
			if !emit(events) {
				return
			}
		}
	}
}
