package e2e

import (
	"context"
	"encoding/json"
	"path/filepath"
	mcp "stsmodel.local/mcpwire"
	"testing"
)

// Apps cooperate only through published MCP messages; no app implementation
// or database model is imported into this cross-application test.
func TestIndependentMCPApplications(t *testing.T) {
	root := repositoryRoot(t)
	ctx := context.Background()
	dir := t.TempDir()
	notes, err := mcp.Start(ctx, filepath.Join(root, "bin/mcp-notes"), []string{"-db", filepath.Join(dir, "notes.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer notes.Close()
	agenda, err := mcp.Start(ctx, filepath.Join(root, "bin/mcp-agenda"), []string{"-db", filepath.Join(dir, "agenda.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer agenda.Close()
	call := func(c *mcp.Client, name string, args json.RawMessage, key string, confirmed bool) mcp.Result {
		t.Helper()
		r, err := c.Call(ctx, name, args, key, confirmed)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	n := call(notes, "notes.create", json.RawMessage(`{"title":"Reunião","content":"2030-05-20T09:00:00-03:00"}`), "note", false)
	if n.IsError {
		t.Fatal(n)
	}
	args := json.RawMessage(`{"title":"Reunião","start":"2030-05-20T09:00:00-03:00","end":"2030-05-20T10:00:00-03:00"}`)
	created := call(agenda, "agenda.create_event", args, "event", false)
	if created.IsError {
		t.Fatal(created)
	}
	agenda.Close()
	agenda, err = mcp.Start(ctx, filepath.Join(root, "bin/mcp-agenda"), []string{"-db", filepath.Join(dir, "agenda.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer agenda.Close()
	replay := call(agenda, "agenda.create_event", args, "event", false)
	if replay.IsError || replay.Content[0].Text != created.Content[0].Text {
		t.Fatal("durable replay changed", replay)
	}
	var payload struct {
		Event struct {
			ID string `json:"id"`
		} `json:"event"`
	}
	if err = json.Unmarshal([]byte(created.Content[0].Text), &payload); err != nil {
		t.Fatal(err)
	}
	cancel, _ := json.Marshal(map[string]string{"id": payload.Event.ID})
	if !call(agenda, "agenda.cancel_event", cancel, "cancel", false).IsError {
		t.Fatal("cancellation bypassed confirmation")
	}
	if call(agenda, "agenda.cancel_event", cancel, "cancel", true).IsError {
		t.Fatal("confirmed cancellation failed")
	}
	// Closing one application does not interrupt the other.
	agenda.Close()
	listed := call(notes, "notes.list", json.RawMessage(`{}`), "", false)
	if listed.IsError || mcp.Status(listed) != "ok" {
		t.Fatal(listed)
	}
}
