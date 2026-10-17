package account

import (
	"context"
	"encoding/json"
	"stsmodel.local/agentwire"
	"time"
)

// Fake uses the same MCP/security path without inference or recorded audio.
func newFake(ctx context.Context, start agentwire.AgentSessionStart, backend *MCP) agentwire.AgentSession {
	ctx, cancel := context.WithCancel(ctx)
	s := &runningSession{cancel: cancel, input: make(chan Input, 8), events: make(chan agentwire.AgentEvent, 8), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer close(s.events)
		emit := func(e agentwire.AgentEvent) bool {
			select {
			case s.events <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !emit(agentwire.AgentEvent{Type: agentwire.AgentSessionReady, Provider: "fake", State: agentwire.Idle}) {
			return
		}
		if start.Principal.Authenticated {
			if !emit(agentwire.AgentEvent{Type: agentwire.OutputTranscript, TurnID: connectedTurn, Role: "ASSISTANT", Stage: "FINAL", Text: connectedMessage}) {
				return
			}
		}
		waiting := !start.Principal.Authenticated
		if waiting {
			for _, event := range welcomeEvents() {
				converted, err := agentEventFromVoice(event)
				if err != nil || !emit(converted) {
					return
				}
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case input := <-s.input:
				if waiting && input.Event.Type == "tool.confirm" && input.Event.Tool != nil && input.Event.Tool.OperationID == welcomeOperation && input.Event.Tool.Approved != nil {
					waiting = false
					if !*input.Event.Tool.Approved {
						emit(agentwire.AgentEvent{Type: agentwire.AgentSessionClosed, State: agentwire.Closed})
						return
					}
					call, c := context.WithTimeout(ctx, 10*time.Second)
					defer c()
					if !start.Principal.Authenticated {
						result, err := backend.Call(call, "identity.begin_login", json.RawMessage(`{}`), start.SessionID+"-login", false)
						if err == nil && !result.IsError && len(result.Content) == 1 {
							var r struct {
								ID string `json:"transaction_id"`
							}
							if json.Unmarshal([]byte(result.Content[0].Text), &r) == nil && r.ID != "" {
								emit(agentwire.AgentEvent{Type: "ui.a2ui", UI: loginSurface(r.ID)})
							}
						}
					}
				}
				if input.Event.Type == "session.stop" {
					emit(agentwire.AgentEvent{Type: agentwire.AgentSessionClosed, State: agentwire.Closed})
					return
				}
			}
		}
	}()
	return s
}
