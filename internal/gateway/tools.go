package gateway

import (
	"context"
	"encoding/json"
	"sync"

	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/orchestrator"
	"stsmodel.local/poc/internal/protocol"
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
}

func (s *Server) openTools(ctx context.Context, namespace string) (*toolRuntime, error) {
	if s.cfg.MCPCommand == "" && s.toolFactory == nil {
		return nil, nil
	}
	toolCtx, cancel := context.WithCancel(ctx)
	var backend orchestrator.Backend
	var closeBackend func()
	var err error
	if s.toolFactory != nil {
		backend, closeBackend, err = s.toolFactory(toolCtx)
	} else {
		var client *mcp.Client
		client, err = mcp.Start(toolCtx, s.cfg.MCPCommand, s.cfg.MCPArgs)
		if err == nil {
			backend = client
			closeBackend = client.Close
		}
	}
	if err != nil {
		cancel()
		return nil, err
	}
	timeout := s.cfg.MCPTimeout
	initCtx, stop := context.WithTimeout(toolCtx, timeout)
	session, err := orchestrator.New(initCtx, backend, s.cfg.MCPAllowedTools, namespace)
	stop()
	if err != nil {
		cancel()
		closeBackend()
		return nil, err
	}
	return &toolRuntime{session: session, ctx: toolCtx, cancel: cancel, closeBackend: closeBackend, done: make(chan toolDone, 4), active: map[string]bool{}, staged: map[string]protocol.Event{}}, nil
}
func (t *toolRuntime) Close() { t.cancel(); t.closeBackend(); t.wg.Wait() }
func (t *toolRuntime) specs() []map[string]any {
	if t == nil {
		return nil
	}
	return t.session.Specs
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
	data, _ := json.Marshal(r)
	return protocol.Event{Type: protocol.ToolResult, TurnID: job.TurnID, Tool: &protocol.Tool{OperationID: job.ID, Name: job.Name, Result: data}}
}
