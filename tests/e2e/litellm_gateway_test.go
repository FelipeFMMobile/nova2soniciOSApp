package e2e

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"stsmodel.local/poc/internal/mcp"
)

func TestLiteLLMLocalGateway(t *testing.T) {
	if os.Getenv("STS_LITELLM_E2E") != "1" {
		t.Skip("set STS_LITELLM_E2E=1 for local Docker validation")
	}
	configs := []mcp.GatewayServerConfig{
		{Alias: "memo", ServerID: "sts-notes", AllowedTools: []string{"notes.create", "notes.list", "notes.delete"}, Policies: map[string]mcp.Policy{"notes.create": mcp.ExplicitIntent, "notes.list": mcp.ReadOnly, "notes.delete": mcp.ConfirmLater}},
		{Alias: "local", ServerID: "sts-agenda", AllowedTools: []string{"agenda.list_slots", "agenda.list_events", "agenda.create_event", "agenda.cancel_event"}, Policies: map[string]mcp.Policy{"agenda.list_slots": mcp.ReadOnly, "agenda.list_events": mcp.ReadOnly, "agenda.create_event": mcp.ExplicitIntent, "agenda.cancel_event": mcp.ConfirmLater}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gateway, err := mcp.NewGateway(ctx, os.Getenv("STS_LITELLM_MCP_URL"), os.Getenv("STS_LITELLM_API_KEY"), os.Getenv("STS_MCP_CONTEXT_SECRET"), configs, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tools, _ := gateway.Tools(ctx)
	if len(tools) != 7 {
		t.Fatalf("discovered %d tools", len(tools))
	}
	createArgs := json.RawMessage(`{"title":"LiteLLM E2E","content":"signed envelope 8042"}`)
	created, err := gateway.Call(ctx, "memo.notes.create", createArgs, "litellm-e2e-create", false)
	if err != nil || created.IsError || mcp.Status(created) != "created" {
		t.Fatal(created, err)
	}
	replayed, err := gateway.Call(ctx, "memo.notes.create", createArgs, "litellm-e2e-create", false)
	if err != nil || replayed.Content[0].Text != created.Content[0].Text {
		t.Fatal(replayed, err)
	}
	var value struct {
		Note struct {
			ID string `json:"id"`
		} `json:"note"`
	}
	if json.Unmarshal([]byte(created.Content[0].Text), &value) != nil || value.Note.ID == "" {
		t.Fatal(created)
	}
	deleteArgs, _ := json.Marshal(map[string]string{"id": value.Note.ID})
	denied, err := gateway.Call(ctx, "memo.notes.delete", deleteArgs, "litellm-e2e-delete", false)
	if err != nil || !denied.IsError {
		t.Fatal("unsigned confirmation accepted", denied, err)
	}
	deleted, err := gateway.Call(ctx, "memo.notes.delete", deleteArgs, "litellm-e2e-delete", true)
	if err != nil || deleted.IsError || mcp.Status(deleted) != "deleted" {
		t.Fatal(deleted, err)
	}
}
