package account

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"stsmodel.local/actionruntime"
	"stsmodel.local/agentwire"
	"stsmodel.local/mcpwire"
	"stsmodel.local/voicewire"
)

type toolDone struct {
	job    orchestrator.Job
	result mcp.Result
}
type toolRuntime struct {
	session      *orchestrator.Session
	ctx          context.Context
	cancel       context.CancelFunc
	closeBackend func()
	done         chan toolDone
	wg           sync.WaitGroup
	active       map[string]bool
	staged       map[string]protocol.Event
	sessionID    string
}

func (s *Runtime) openTools(ctx context.Context, namespace, sessionID string) (*toolRuntime, error) {
	toolCtx, cancel := context.WithCancel(ctx)
	backend, closeBackend, err := s.toolFactory(toolCtx)
	if err != nil {
		cancel()
		return nil, err
	}
	allowed := []string{}
	for name := range policies {
		allowed = append(allowed, name)
	}
	timeout := s.cfg.MCPTimeout
	initCtx, stop := context.WithTimeout(toolCtx, timeout)
	session, err := orchestrator.New(initCtx, backend, allowed, namespace, policies)
	stop()
	if err != nil {
		cancel()
		closeBackend()
		return nil, err
	}
	runtime := &toolRuntime{session: session, ctx: toolCtx, cancel: cancel, closeBackend: closeBackend, done: make(chan toolDone, 4), active: map[string]bool{}, staged: map[string]protocol.Event{}, sessionID: sessionID}
	return runtime, nil
}
func (t *toolRuntime) Close() {
	t.cancel()
	t.closeBackend()
	t.wg.Wait()

}
func (t *toolRuntime) specs() []agentwire.ToolSpec {
	if t == nil {
		return nil
	}
	return t.session.Specs
}
func pendingJob(p *orchestrator.Pending) orchestrator.Job {
	return orchestrator.Job{ID: p.OperationID, Name: p.Name, TurnID: p.TurnID, Args: p.Args}
}
func cancelledResult() mcp.Result {
	return mcp.TextResult(map[string]string{"status": "cancelled"}, false)
}

// Called by the UI control path; the orchestrator still binds approval to the
// pending operation and exact arguments. Cancellation never executes a job.
func (t *toolRuntime) confirm(id string, approved bool) (orchestrator.Job, *mcp.Result, bool) {
	if t == nil || t.session.Pending == nil {
		return orchestrator.Job{}, nil, false
	}
	pending := t.session.Pending
	if !t.session.Confirm(id, approved) {
		return orchestrator.Job{}, nil, false
	}
	if !approved {
		result := cancelledResult()
		return pendingJob(pending), &result, true
	}
	job, result := t.session.Plan(pending.OperationID, strings.ReplaceAll(pending.Name, ".", "_"), pending.TurnID, pending.Args)
	return job, result, true
}
func (t *toolRuntime) start(job orchestrator.Job, run func(context.Context, orchestrator.Job) mcp.Result) {
	t.active[job.ID] = true
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		result := run(t.ctx, job)
		select {
		case t.done <- toolDone{job, result}:
		case <-t.ctx.Done():
		}
	}()
}
func toolResultEvent(job orchestrator.Job, r mcp.Result) protocol.Event {
	status := "completed"
	if len(r.Content) == 1 {
		var value struct {
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(r.Content[0].Text), &value) == nil && value.Status == "cancelled" {
			status = "cancelled"
		}
	}
	if status == "completed" && job.Name == "identity.logout" && !r.IsError {
		status = "signed_out"
	}
	data, _ := json.Marshal(mcp.TextResult(map[string]string{"status": status}, r.IsError))
	return protocol.Event{Type: protocol.ToolResult, TurnID: job.TurnID, Tool: &protocol.Tool{OperationID: job.ID, Name: job.Name, Result: data}}
}
