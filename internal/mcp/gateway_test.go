package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(value any) *http.Response {
	data, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header)}
}

func TestGatewayDiscoversBusinessSchemaAndSignsCalls(t *testing.T) {
	codec, _ := NewEnvelopeCodec(testSecret, "memo")
	wrapped := EnvelopeTools(NotesTools())[0].InputSchema
	var auth, serverID, callName string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		auth = r.Header.Get("Authorization")
		if r.Method == http.MethodGet {
			serverID = r.URL.Query().Get("server_id")
			return jsonResponse(map[string]any{"tools": []map[string]any{{"name": "notes.create", "description": "create", "inputSchema": json.RawMessage(wrapped)}}}), nil
		}
		var body struct {
			ServerID  string          `json:"server_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		serverID, callName = body.ServerID, body.Name
		payload, key, confirmed, err := codec.Unwrap(body.Name, body.Arguments)
		if err != nil || key != "operation" || confirmed || string(payload) != `{"content":"c","title":"t"}` {
			t.Errorf("bad envelope: %s %s %v %v", payload, key, confirmed, err)
		}
		return jsonResponse(TextResult(map[string]string{"status": "created"}, false)), nil
	})}
	configs := []GatewayServerConfig{{Alias: "memo", ServerID: "notes-id", AllowedTools: []string{"notes.create"}, Policies: map[string]Policy{"notes.create": ExplicitIntent}}}
	gateway, err := newGateway(context.Background(), "http://litellm.test", "service-key", testSecret, configs, client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	tools, _ := gateway.Tools(context.Background())
	if len(tools) != 1 || tools[0].Name != "memo.notes.create" || string(tools[0].InputSchema) != string(NotesTools()[0].InputSchema) {
		t.Fatal(tools)
	}
	result, err := gateway.Call(context.Background(), "memo.notes.create", json.RawMessage(`{"title":"t","content":"c"}`), "operation", false)
	if err != nil || result.IsError || auth != "Bearer service-key" || serverID != "notes-id" || callName != "notes.create" {
		t.Fatal(result, err, auth, serverID, callName)
	}
}

func TestGatewayRejectsMissingAdvertisedTool(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(map[string]any{"tools": []any{}}), nil
	})}
	configs := []GatewayServerConfig{{Alias: "memo", ServerID: "notes-id", AllowedTools: []string{"notes.create"}, Policies: map[string]Policy{"notes.create": ExplicitIntent}}}
	if _, err := newGateway(context.Background(), "http://litellm.test", "key", testSecret, configs, client, time.Second); err == nil {
		t.Fatal("missing tool accepted")
	}
}
