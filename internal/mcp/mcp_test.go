package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"stsmodel.local/poc/internal/storage"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("STS_MCP_TEST_HELPER") != "1" {
		return
	}
	if os.Getenv("STS_MCP_TEST_HANG") == "1" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	s, err := storage.Open(os.Getenv("STS_MCP_TEST_DB"))
	if err != nil {
		os.Exit(2)
	}
	var codec *EnvelopeCodec
	if os.Getenv("STS_MCP_TEST_ENVELOPE") == "1" {
		codec, err = NewEnvelopeCodec(os.Getenv("STS_MCP_CONTEXT_SECRET"), "memo")
	}
	if err == nil {
		err = Serve(context.Background(), os.Stdin, os.Stdout, s, codec)
	}
	s.Close()
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestEnvelopeModeOverRealStdio(t *testing.T) {
	t.Setenv("STS_MCP_TEST_HELPER", "1")
	t.Setenv("STS_MCP_TEST_ENVELOPE", "1")
	t.Setenv("STS_MCP_CONTEXT_SECRET", testSecret)
	t.Setenv("STS_MCP_TEST_DB", filepath.Join(t.TempDir(), "envelope.sqlite"))
	c, err := Start(context.Background(), os.Args[0], []string{"-test.run=^TestHelperProcess$"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tools, err := c.Tools(ctx)
	if err != nil || len(tools) != 3 {
		t.Fatal(tools, err)
	}
	codec, _ := NewEnvelopeCodec(testSecret, "memo")
	payload := json.RawMessage(`{"title":"Envelope","content":"durable"}`)
	envelope, _ := codec.Wrap("notes.create", payload, "stable-envelope", false)
	var result Result
	if err = c.request(ctx, "tools/call", map[string]any{"name": "notes.create", "arguments": envelope}, &result); err != nil || result.IsError {
		t.Fatal(result, err)
	}
	// Legacy _meta and plain payload cannot bypass signed-envelope mode.
	if err = c.request(ctx, "tools/call", map[string]any{"name": "notes.create", "arguments": payload, "_meta": map[string]any{"sts/idempotencyKey": "forged", "sts/confirmed": true}}, &result); err != nil || !result.IsError || Status(result) != "error" {
		t.Fatal("unsigned call accepted", result, err)
	}
}

func TestStdioDiscoveryCallsAndApproval(t *testing.T) {
	t.Setenv("STS_MCP_TEST_HELPER", "1")
	t.Setenv("STS_MCP_TEST_DB", filepath.Join(t.TempDir(), "test.sqlite"))
	c, err := Start(context.Background(), os.Args[0], []string{"-test.run=^TestHelperProcess$"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tools, err := c.Tools(ctx)
	if err != nil || len(tools) != 3 {
		t.Fatal(tools, err)
	}
	args := json.RawMessage(`{"title":"Teste","content":"MCP exclusivo 5927"}`)
	created, err := c.Call(ctx, "notes.create", args, "stable-request", false)
	if err != nil || created.IsError {
		t.Fatal(created, err)
	}
	replay, err := c.Call(ctx, "notes.create", args, "stable-request", false)
	if err != nil || replay.Content[0].Text != created.Content[0].Text {
		t.Fatal(replay, err)
	}
	invalid, err := c.Call(ctx, "notes.create", json.RawMessage(`{"title":"x","content":"y","confirmed":true}`), "invalid", false)
	if err != nil || !invalid.IsError {
		t.Fatal("schema accepted", invalid, err)
	}
	var value struct {
		Note storage.Note `json:"note"`
	}
	if err = json.Unmarshal([]byte(created.Content[0].Text), &value); err != nil {
		t.Fatal(err)
	}
	deletion, _ := json.Marshal(map[string]string{"id": value.Note.ID})
	denied, err := c.Call(ctx, "notes.delete", deletion, "delete-key", false)
	if err != nil || !denied.IsError {
		t.Fatal("unapproved deletion", denied, err)
	}
	listed, err := c.Call(ctx, "notes.list", json.RawMessage(`{"query":"5927"}`), "", false)
	if err != nil || listed.IsError {
		t.Fatal(listed, err)
	}
	removed, err := c.Call(ctx, "notes.delete", deletion, "delete-key", true)
	if err != nil || removed.IsError {
		t.Fatal(removed, err)
	}
}

func TestTimeoutClosesProcess(t *testing.T) {
	t.Setenv("STS_MCP_TEST_HELPER", "1")
	t.Setenv("STS_MCP_TEST_HANG", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	c, err := Start(ctx, os.Args[0], []string{"-test.run=^TestHelperProcess$"})
	if c != nil {
		c.Close()
	}
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not release child", err)
	}
}

func TestNoRemoteSchemaAndRequiredArguments(t *testing.T) {
	if _, err := Compile(json.RawMessage(`{"$ref":"https://example.com/private"}`)); err == nil {
		t.Fatal("remote schema allowed")
	}
	schema, err := Compile(NotesTools()[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if Validate(schema, json.RawMessage(`{"title":"missing content"}`)) == nil {
		t.Fatal("required arguments accepted")
	}
}
