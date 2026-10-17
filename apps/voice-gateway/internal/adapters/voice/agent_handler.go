package gateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/agentwire"
	"stsmodel.local/voicewire"
)

// handleVoice owns only the public WebSocket lifecycle and translation to the
// AgentSession port. Model, semantic history, tools and confirmation execute
// behind the configured factory.
func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
	version := 1
	if r.URL.Path == "/v2/voice" {
		version = 2
	}
	principal, ok := r.Context().Value(principalContextKey{}).(agentwire.Principal)
	if !ok || agentwire.ValidatePrincipal(principal) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authenticated principal missing"})
		return
	}
	if version == 2 {
		var authErr error
		principal, authErr = s.accountPrincipal(r)
		if authErr != nil {
			writeJSON(w, 401, map[string]string{"error": "authentication_required"})
			return
		}
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session limit reached"})
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: s.cfg.ReadTimeout, ReadBufferSize: 4096, WriteBufferSize: 4096}
	conn, err := upgrader.Upgrade(w, r, nil)
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
	in := make(chan inbound, 8)
	readDone := make(chan struct{})
	var agent agentwire.AgentSession
	defer func() {
		cancel()
		if agent != nil {
			_ = agent.Close()
		}
		_ = conn.Close()
		<-readDone
	}()
	conn.SetReadLimit(s.cfg.MaxEventBytes)
	_ = conn.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(s.cfg.IdleTimeout)) })
	go readVoiceEvents(ctx, cancel, conn, in, readDone, s.cfg.IdleTimeout, version)

	sessionID := rand.Text()
	write := func(event protocol.Event) error {
		event.Version = version
		event.SessionID = sessionID
		_ = conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
		return conn.WriteJSON(event)
	}
	sendError := func(code, message string) error {
		s.metrics.Errors.Add(1)
		return write(protocol.Event{Type: protocol.Error, Code: code, Message: message})
	}
	startTimer := time.NewTimer(s.cfg.ReadTimeout)
	defer startTimer.Stop()
	ping := time.NewTicker(s.cfg.IdleTimeout / 3)
	defer ping.Stop()

start:
	for {
		select {
		case <-ctx.Done():
			return
		case <-startTimer.C:
			_ = sendError("session_timeout", "session.start was not received in time")
			return
		case <-ping.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(s.cfg.WriteTimeout)) != nil {
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
			if input.event.Type != protocol.SessionStart {
				if sendError("session_required", "first event must be session.start") != nil {
					return
				}
				continue
			}
			if input.event.Provider != "" && input.event.Provider != s.cfg.Provider {
				if sendError("provider_unavailable", "requested provider is not selected") != nil {
					return
				}
				continue
			}
			selected := s.cfg.DefaultAgentID
			if version == 2 && input.event.AgentID != "" {
				selected = input.event.AgentID
			}
			selectedPrincipal := principal
			found := false
			for _, endpoint := range s.cfg.AgentCatalog {
				if endpoint.AgentID == selected {
					found = true
					if version == 1 && endpoint.Protocol == agentwire.ProtocolV2 {
						found = false
					}
					if endpoint.Protocol != agentwire.ProtocolV2 {
						selectedPrincipal = s.cfg.Principal
					}
				}
			}
			if !found {
				_ = sendError("agent_unavailable", "agent is not available")
				return
			}
			agent, err = s.agentFactory.Open(ctx, agentwire.AgentSessionStart{SessionID: sessionID, RequestID: input.event.RequestID, AgentID: selected, Provider: s.cfg.Provider, Principal: selectedPrincipal})
			if err != nil {
				_ = sendError("provider_unavailable", "agent session could not start")
				return
			}
			startTimer.Stop()
			break start
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(s.cfg.WriteTimeout)) != nil {
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
			if input.event.SessionID != sessionID {
				if sendError("session_mismatch", "sessionId does not match this connection") != nil {
					return
				}
				continue
			}
			command, err := agentCommandFromVoice(input.event)
			if err != nil {
				if sendError("invalid_transition", err.Error()) != nil {
					return
				}
				continue
			}
			if err := agent.Send(ctx, command); err != nil {
				_ = sendError("provider_unavailable", "agent session disconnected")
				return
			}
		case event, ok := <-agent.Events():
			if !ok {
				return
			}
			{
				switch event.Type {
				case agentwire.TurnCompleted:
					s.metrics.Completed.Add(1)
					if event.Metrics != nil && event.Metrics.FirstAudioMS > 0 {
						s.metrics.recordFirstAudio(time.Duration(event.Metrics.FirstAudioMS * float64(time.Millisecond)))
					}
				case agentwire.TurnInterrupted:
					s.metrics.Interrupted.Add(1)
				case agentwire.AgentError:
					s.metrics.Errors.Add(1)
				}
			}
			voiceEvent, err := voiceEventFromAgent(event)
			if err != nil {
				_ = sendError("provider_failed", "agent returned an invalid event")
				return
			}
			if write(voiceEvent) != nil {
				return
			}
			if event.Type == agentwire.AgentSessionClosed {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session stopped"), time.Now().Add(s.cfg.WriteTimeout))
				return
			}
		}
	}
}

func readVoiceEvents(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, in chan<- inbound, done chan<- struct{}, idleTimeout time.Duration, version int) {
	defer cancel()
	defer close(done)
	defer close(in)
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(idleTimeout))
		event, decodeErr := protocol.DecodeClientVersion(data, version)
		if decodeErr == nil && event.Version != version {
			decodeErr = errors.New("protocol version mismatch")
		}
		if kind != websocket.TextMessage {
			decodeErr = errors.New("only JSON text frames are accepted")
		}
		select {
		case in <- inbound{event: event, err: decodeErr}:
		case <-ctx.Done():
			return
		}
	}
}

func agentCommandFromVoice(event protocol.Event) (agentwire.AgentCommand, error) {
	command := agentwire.AgentCommand{SessionID: event.SessionID, TurnID: event.TurnID, Sequence: event.Sequence}
	switch event.Type {
	case protocol.AudioAppend:
		pcm, err := protocol.DecodeAudio(event.Audio)
		if err != nil {
			return agentwire.AgentCommand{}, err
		}
		command.Type = agentwire.InputAudio
		command.Audio = pcm
	case protocol.TurnCommit:
		command.Type = agentwire.InputCommit
	case protocol.TurnCancel:
		command.Type = agentwire.InputCancel
	case protocol.ToolConfirm:
		command.Type = agentwire.InputConfirmation
		command.OperationID = event.Tool.OperationID
		command.Approved = *event.Tool.Approved
	case protocol.SessionStop:
		command.Type = agentwire.InputClose
	default:
		return agentwire.AgentCommand{}, errors.New("session is already started")
	}
	return command, nil
}

func voiceEventFromAgent(event agentwire.AgentEvent) (protocol.Event, error) {
	converted := protocol.Event{
		TurnID: event.TurnID, Sequence: event.Sequence, Provider: event.Provider,
		State: event.State, SampleRate: event.SampleRate, Text: event.Text,
		Role: event.Role, Stage: event.Stage, Code: event.Code, Message: event.Message,
	}
	switch event.Type {
	case agentwire.AgentSessionReady:
		converted.Type = protocol.SessionReady
	case agentwire.AgentSessionState:
		converted.Type = protocol.SessionState
	case agentwire.AgentSessionRenewed:
		converted.Type = protocol.SessionRenewed
	case agentwire.AgentSessionClosed:
		converted.Type = protocol.SessionStopped
	case "ui.a2ui":
		converted.Type = "ui.a2ui"
		converted.UI = event.UI
	case agentwire.OutputAudio:
		if len(event.Audio) == 0 {
			return protocol.Event{}, errors.New("empty agent audio")
		}
		converted.Type = protocol.AudioOutput
		converted.Audio = base64.StdEncoding.EncodeToString(event.Audio)
	case agentwire.OutputTranscript:
		converted.Type = protocol.Transcript
	case agentwire.TurnStarted:
		converted.Type = protocol.TurnStarted
	case agentwire.TurnCompleted:
		converted.Type = protocol.TurnCompleted
	case agentwire.TurnInterrupted:
		converted.Type = protocol.TurnInterrupted
	case agentwire.ToolStarted:
		converted.Type = protocol.ToolStarted
	case agentwire.ToolResult:
		converted.Type = protocol.ToolResult
	case agentwire.ToolConfirmation:
		converted.Type = protocol.ToolConfirmation
	case agentwire.AgentError:
		converted.Type = protocol.Error
	default:
		return protocol.Event{}, errors.New("unsupported agent event")
	}
	if event.Metrics != nil {
		converted.Metrics = &protocol.Metrics{FirstAudioMS: event.Metrics.FirstAudioMS, DurationMS: event.Metrics.DurationMS, InterruptionMS: event.Metrics.InterruptionMS}
	}
	if event.Tool != nil {
		converted.Tool = &protocol.Tool{OperationID: event.Tool.OperationID, Name: event.Tool.Name, Arguments: event.Tool.Arguments, Result: event.Tool.Result}
	}
	return converted, nil
}
