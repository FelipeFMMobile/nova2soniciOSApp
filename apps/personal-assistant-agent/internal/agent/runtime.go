package agent

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"stsmodel.local/agentwire"
	"stsmodel.local/mcpwire"
	agentconfig "stsmodel.local/personal-assistant-agent/config"
	"stsmodel.local/personal-assistant-agent/internal/actions"
	"stsmodel.local/personal-assistant-agent/internal/adapters/nova"
	"stsmodel.local/voicewire"
)

type Input struct {
	Event protocol.Event
	Err   error
}

type Hooks struct {
	Error       func()
	Completed   func()
	Interrupted func()
	FirstAudio  func(time.Duration)
}

type ToolFactory func(context.Context) (orchestrator.Backend, func(), error)

type Runtime struct {
	cfg         agentconfig.Config
	prompt      string
	logger      *slog.Logger
	hooks       Hooks
	toolFactory ToolFactory
}

func New(cfg agentconfig.Config, logger *slog.Logger, hooks Hooks, toolFactory ToolFactory) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	prompt, err := agentconfig.LoadPrompt(cfg.PromptFile)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{cfg: cfg, prompt: prompt, logger: logger, hooks: hooks, toolFactory: toolFactory}, nil
}

func call(fn func()) {
	if fn != nil {
		fn()
	}
}

// Run owns all Nova state and writes. Microphone input remains live while
// inference responds; only the private stream reader runs asynchronously.
func (s *Runtime) Run(ctx context.Context, in <-chan Input, sessionID, namespace string, write func(protocol.Event) error) {
	sendError := func(code, message string) {
		call(s.hooks.Error)
		_ = write(protocol.Event{Type: protocol.Error, Code: code, Message: message})
	}
	tools, err := s.openTools(ctx, namespace, sessionID)
	if err != nil {
		sendError("mcp_unavailable", "MCP initialization or discovery failed")
		return
	}
	if tools != nil {
		defer tools.Close()
	}
	stream, err := nova.Open(ctx, s.cfg.LiteLLMURL, s.cfg.LiteLLMAPIKey, s.cfg.LiteLLMModel, sessionID, namespace, s.prompt, nil, tools.specs())
	if err != nil {
		sendError("provider_unavailable", err.Error())
		return
	}
	defer func() { stream.Close() }()
	mapper := nova.NewMapper(nil)
	if write(protocol.Event{Type: protocol.SessionReady, Provider: "nova", State: agentwire.Idle}) != nil {
		return
	}
	inputID := ""
	inputSequence := uint64(0)
	usedInputs := map[string]bool{}
	openedAt := time.Now()
	renew := time.NewTimer(s.cfg.NovaMaxSessionAge)
	defer renew.Stop()
	turnTimer := time.NewTimer(time.Hour)
	turnTimer.Stop()
	defer turnTimer.Stop()
	var turnTimeout <-chan time.Time
	var toolResults <-chan toolDone
	if tools != nil {
		toolResults = tools.done
	}
	userTexts := map[string]string{}
	userTurns := map[string]string{}
	completedUser := ""
	completedUserTurn := ""
	emit := func(events []protocol.Event) bool {
		for _, event := range events {
			if event.Type == protocol.SessionState && event.State == agentwire.Responding && turnTimeout == nil {
				turnTimer.Reset(s.cfg.TurnTimeout)
				turnTimeout = turnTimer.C
			}
			if event.Type == protocol.AudioOutput && event.Sequence == 1 {
				if s.hooks.FirstAudio != nil {
					s.hooks.FirstAudio(time.Duration(mapper.FirstAudioMicros()) * time.Microsecond)
				}
			}
			if event.Type == protocol.TurnInterrupted {
				call(s.hooks.Interrupted)
				turnTimer.Stop()
				turnTimeout = nil
			}
			if event.Type == protocol.TurnCompleted {
				call(s.hooks.Completed)
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
		if tools != nil {
			tools.session.Interrupt()
			clear(tools.staged)
		}
		replacement, err := nova.Open(ctx, s.cfg.LiteLLMURL, s.cfg.LiteLLMAPIKey, s.cfg.LiteLLMModel, sessionID, namespace, s.prompt, history, tools.specs())
		if err != nil {
			sendError("provider_unavailable", "Nova session renewal failed")
			return false
		}
		stream = replacement
		mapper = nova.NewMapper(history)
		openedAt = time.Now()
		renew.Reset(s.cfg.NovaMaxSessionAge)
		return write(protocol.Event{Type: protocol.SessionRenewed, Provider: "nova", State: agentwire.Idle}) == nil
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-turnTimeout:
			if !emit(mapper.Interrupt()) {
				return
			}
			sendError("provider_timeout", "Nova did not complete the turn in time")
			return
		case <-renew.C:
			if tools != nil && len(tools.active) > 0 {
				if time.Since(openedAt) >= 7*time.Minute+50*time.Second {
					sendError("mcp_in_progress", "session limit reached while tool runs; retry with the same requestId")
					return
				}
				renew.Reset(10 * time.Second)
				continue
			}
			if mapper.State != agentwire.Idle && time.Since(openedAt) < 7*time.Minute+50*time.Second {
				renew.Reset(10 * time.Second)
				continue
			}
			if mapper.State != agentwire.Idle && !emit(mapper.Interrupt()) {
				return
			}
			if !rotate() {
				return
			}
		case input, ok := <-in:
			if !ok {
				return
			}
			if input.Err != nil {
				sendError("invalid_event", input.Err.Error())
				continue
			}
			e := input.Event
			if e.SessionID != sessionID {
				sendError("session_mismatch", "sessionId does not match this connection")
				continue
			}
			switch e.Type {
			case protocol.AudioAppend:
				if e.TurnID != inputID {
					if e.Sequence != 1 || usedInputs[e.TurnID] || len(usedInputs) >= agentwire.MaxTurns {
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
				if e.TurnID != mapper.TurnID || mapper.State == agentwire.Idle {
					sendError("invalid_turn", "no matching active turn")
					continue
				}
				if !emit(mapper.Interrupt()) {
					return
				}
				if tools != nil && len(tools.active) > 0 {
					sendError("mcp_in_progress", "output cancelled; operation outcome may be pending, retry with the same requestId")
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
				_ = write(protocol.Event{Type: protocol.SessionStopped, State: agentwire.Closed})
				return
			case protocol.ToolConfirm:
				if tools == nil || !tools.session.Confirm(e.Tool.OperationID, *e.Tool.Approved) {
					sendError("invalid_confirmation", "no matching pending operation")
				}
			default:
				sendError("invalid_transition", "session is already started")
			}
		case finished := <-toolResults:
			delete(tools.active, finished.job.ID)
			tools.session.Complete(finished.job, finished.result)
			if tools.evidenceResult(finished.job, finished.result) != nil {
				sendError("storage_failed", "cannot persist private evidence")
				return
			}
			if stream.SendToolResult(finished.job.ID, finished.result) != nil {
				sendError("provider_unavailable", "Nova tool result delivery failed; retry mutations with the same requestId")
				return
			}
			if !emit([]protocol.Event{toolResultEvent(finished.job, finished.result)}) {
				return
			}
			s.logger.Info("MCP operation completed", "operation_id", finished.job.ID, "tool", finished.job.Name, "failed", finished.result.IsError)
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
			if tools != nil {
				// Approval observes only the provider's finalized USER transcript,
				// after all fragments have arrived, never speculative text.
				for _, e := range events {
					if e.Type == protocol.Transcript && e.Role == "USER" && e.Stage == "FINAL" {
						if len(userTexts[event.ContentID])+len(e.Text) > 8192 {
							sendError("provider_failed", "user transcript too large")
							return
						}
						userTexts[event.ContentID] += e.Text
						userTurns[event.ContentID] = e.TurnID
					}
				}
				if event.Kind == "contentEnd" {
					if text, ok := userTexts[event.ContentID]; ok {
						turn := userTurns[event.ContentID]
						if completedUserTurn != turn {
							completedUser = ""
							completedUserTurn = turn
						}
						completedUser = strings.TrimSpace(completedUser + " " + text)
						// USER ASR content commonly ends with PARTIAL_TURN. The
						// subsequent TOOL/ASSISTANT content is the utterance boundary.
						delete(userTexts, event.ContentID)
						delete(userTurns, event.ContentID)
					}
				}
				if event.Kind == "contentStart" && event.Role != "USER" && completedUserTurn != "" && len(userTexts) == 0 {
					tools.session.ObserveUser(completedUserTurn, completedUser)
				}
				if event.Kind == "toolUse" {
					if len(tools.staged) >= 32 {
						sendError("provider_failed", "too many staged tool calls")
						return
					}
					for _, e := range events {
						if e.Type == protocol.ToolStarted {
							tools.staged[event.ContentID] = e
						}
					}
					if _, ok := tools.staged[event.ContentID]; !ok {
						tools.staged[event.ContentID] = protocol.Event{Type: protocol.ToolStarted, TurnID: mapper.TurnID, Code: "turn_cancelled", Tool: &protocol.Tool{OperationID: event.ToolID, Name: event.ToolName, Arguments: event.ToolArguments}}
					}
					events = nil // Execute only after Nova closes its TOOL content.
				}
				if event.Kind == "contentEnd" {
					if call, ok := tools.staged[event.ContentID]; ok {
						delete(tools.staged, event.ContentID)
						if tools.active[call.Tool.OperationID] {
							continue
						}
						var job orchestrator.Job
						var immediate *mcp.Result
						if call.Code == "turn_cancelled" || call.TurnID != mapper.TurnID {
							job = orchestrator.Job{ID: call.Tool.OperationID, Name: call.Tool.Name, TurnID: call.TurnID}
							r := mcp.Failure("turn_cancelled")
							immediate = &r
						} else {
							job, immediate = tools.session.Plan(call.Tool.OperationID, call.Tool.Name, call.TurnID, call.Tool.Arguments)
						}
						if !emit([]protocol.Event{call}) {
							return
						}
						if immediate == nil && len(tools.active) >= 4 {
							r := mcp.Failure("tool_busy")
							immediate = &r
						}
						if immediate != nil {
							if tools.evidenceResult(job, *immediate) != nil {
								sendError("storage_failed", "cannot persist private evidence")
								return
							}
							if stream.SendToolResult(job.ID, *immediate) != nil {
								sendError("provider_unavailable", "Nova tool result delivery failed")
								return
							}
							if !emit([]protocol.Event{toolResultEvent(job, *immediate)}) {
								return
							}
							if p := tools.session.Pending; p != nil {
								if !emit([]protocol.Event{{Type: protocol.ToolConfirmation, TurnID: call.TurnID, Tool: &protocol.Tool{OperationID: p.OperationID, Name: p.Name, Arguments: p.Args}, Message: "Diga exatamente " + tools.session.ConfirmationPhrase() + ", em um novo turno, para o alvo exibido."}}) {
									return
								}
							}
						} else {
							tools.start(job, func(parent context.Context, j orchestrator.Job) mcp.Result {
								deadline, cancel := context.WithTimeout(parent, s.cfg.MCPTimeout)
								defer cancel()
								return tools.session.Execute(deadline, j)
							})
						}
					}
				}
			}
			if !emit(events) {
				return
			}
		}
	}
}
