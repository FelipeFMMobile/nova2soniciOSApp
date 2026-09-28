package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	mcp "stsmodel.local/mcpwire"
	"testing"
)

type configuredBackend struct{}

func (configuredBackend) Tools(context.Context) ([]mcp.Tool, error) {
	return []mcp.Tool{{Name: "inventory.remove", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`), Annotations: map[string]any{"readOnlyHint": true}}}, nil
}
func (configuredBackend) Call(context.Context, string, json.RawMessage, string, bool) (mcp.Result, error) {
	return mcp.TextResult(map[string]string{"status": "removed"}, false), nil
}
func TestNewApplicationUsesTrustedConfiguration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STS_INTEGRATIONS_DIR", dir)
	if _, err := New(context.Background(), configuredBackend{}, []string{"inventory.remove"}, "s"); err == nil {
		t.Fatal("remote annotation authorized unknown tool")
	}
	profile := `{"tools":{"inventory.remove":{"policy":"confirm_later","confirmation_phrase":"confirmo remover item"}}}`
	if err := os.WriteFile(filepath.Join(dir, "inventory.json"), []byte(profile), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), configuredBackend{}, []string{"inventory.remove"}, "s")
	if err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"id":"sku-1"}`)
	_, result := s.Plan("op1", "inventory_remove", "turn1", args)
	if result == nil || mcp.Status(*result) != "confirmation_required" {
		t.Fatal(result)
	}
	if s.ObserveUser("turn1", "confirmo remover item") {
		t.Fatal("same turn approved")
	}
	if s.Confirm("op1", true) {
		t.Fatal("UI bypassed voice-only policy")
	}
	if !s.ObserveUser("turn2", "confirmo remover item") {
		t.Fatal("configured phrase ignored")
	}
	job, result := s.Plan("op2", "inventory_remove", "turn2", args)
	if result != nil || !job.Confirmed {
		t.Fatal(job, result)
	}
	if mcp.Status(s.Execute(context.Background(), job)) != "removed" {
		t.Fatal("configured operation failed")
	}
}
