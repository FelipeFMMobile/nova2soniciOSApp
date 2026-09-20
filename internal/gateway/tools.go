package gateway

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/orchestrator"
	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/provider"
)

type toolDone struct {
	job    orchestrator.Job
	result mcp.Result
}
type toolRuntime struct {
	evidence     *os.File
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

func (s *Server) openTools(ctx context.Context, namespace, sessionID string) (*toolRuntime, error) {
	if s.cfg.MCPBackend != "litellm" && s.cfg.MCPCommand == "" && len(s.cfg.MCPServers) == 0 && s.toolFactory == nil {
		return nil, nil
	}
	toolCtx, cancel := context.WithCancel(ctx)
	var backend orchestrator.Backend
	var closeBackend func()
	var err error
	allowed := s.cfg.MCPAllowedTools
	if s.toolFactory != nil {
		backend, closeBackend, err = s.toolFactory(toolCtx)
	} else if s.cfg.MCPBackend == "litellm" {
		var gateway *mcp.Gateway
		gateway, err = mcp.NewGateway(toolCtx, s.cfg.LiteLLMURL, s.cfg.LiteLLMAPIKey, s.cfg.MCPContextSecret, s.cfg.LiteLLMServers, s.cfg.MCPTimeout)
		if err == nil {
			backend = gateway
			closeBackend = func() {}
			allowed = gateway.AllowedTools()
		}
	} else if len(s.cfg.MCPServers) > 0 {
		var router *mcp.Router
		router, err = mcp.StartServers(toolCtx, s.cfg.MCPServers, s.cfg.MCPTimeout)
		if err == nil {
			backend = router
			closeBackend = router.Close
			allowed = router.AllowedTools()
		}
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
	session, err := orchestrator.New(initCtx, backend, allowed, namespace)
	stop()
	if err != nil {
		cancel()
		closeBackend()
		return nil, err
	}
	runtime := &toolRuntime{session: session, ctx: toolCtx, cancel: cancel, closeBackend: closeBackend, done: make(chan toolDone, 4), active: map[string]bool{}, staged: map[string]protocol.Event{}, sessionID: sessionID}
	runtime.evidence, err = openEvidence(s.cfg.MCPEvidencePath)
	if err != nil {
		runtime.Close()
		return nil, err
	}
	return runtime, nil
}
func (t *toolRuntime) Close() {
	t.cancel()
	t.closeBackend()
	t.wg.Wait()
	if t.evidence != nil {
		t.evidence.Close()
	}
}
func (t *toolRuntime) specs() []provider.ToolSpec {
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
