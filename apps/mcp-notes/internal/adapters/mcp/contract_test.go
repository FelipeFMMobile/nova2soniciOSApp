package adapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	mcp "stsmodel.local/mcpwire"
	"testing"
)

func TestPublishedResults(t *testing.T) {
	raw, err := os.ReadFile("../../../contracts/v1/results.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := mcp.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	check := func(raw json.RawMessage) {
		t.Helper()
		if err := mcp.Validate(schema, raw); err != nil {
			t.Fatal(string(raw), err)
		}
	}
	examples, err := os.ReadFile("../../../contracts/v1/examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var values []json.RawMessage
	if err = json.Unmarshal(examples, &values); err != nil {
		t.Fatal(err)
	}
	for _, v := range values {
		check(v)
	}
	t.Setenv("STS_MCP_TEST_HELPER", "1")
	t.Setenv("STS_MCP_TEST_DB", filepath.Join(t.TempDir(), "contract.sqlite"))
	c, err := mcp.Start(context.Background(), os.Args[0], []string{"-test.run=^TestHelperProcess$"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	call := func(name string, args json.RawMessage, key string, confirmed bool) mcp.Result {
		t.Helper()
		r, err := c.Call(context.Background(), name, args, key, confirmed)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Content) == 0 {
			t.Fatal("empty result")
		}
		check(json.RawMessage(r.Content[0].Text))
		return r
	}
	created := call("notes.create", json.RawMessage(`{"title":"ação","content":"olá"}`), "create", false)
	if created.IsError {
		t.Fatal(created)
	}
	var result map[string]json.RawMessage
	_ = json.Unmarshal([]byte(created.Content[0].Text), &result)
	var item struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(result["note"], &item)
	target, _ := json.Marshal(map[string]string{"id": item.ID})
	denied := call("notes.delete", target, "delete", false)
	if !denied.IsError {
		t.Fatal("unconfirmed mutation")
	}
	removed := call("notes.delete", target, "delete", true)
	if removed.IsError {
		t.Fatal(removed)
	}
}
