package account

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"stsmodel.local/agentwire"
	"stsmodel.local/mcpwire"
	"sync/atomic"
)

type MCP struct {
	url, key, session, protocol string
	client                      *http.Client
	sequence                    atomic.Uint64
	tools                       []mcp.Tool
	routes                      map[string]string
	codecs                      map[string]*mcp.EnvelopeCodec
	identity                    mcp.IdentityContext
}

func NewMCP(ctx context.Context, c Config, p agentwire.Principal) (*MCP, error) {
	scopes := []string{}
	if p.Authenticated {
		scopes = []string{"profile:read", "profile:write"}
	}
	b := &MCP{url: strings.TrimRight(c.LiteLLMURL, "/") + "/mcp", key: c.LiteLLMAPIKey, protocol: "2025-06-18", client: &http.Client{Timeout: c.MCPTimeout}, routes: map[string]string{}, codecs: map[string]*mcp.EnvelopeCodec{}, identity: mcp.IdentityContext{Subject: p.Subject, ClientCode: p.ClientCode, SessionRef: p.SessionRef, DeviceID: p.DeviceID, Scopes: scopes}}
	for _, audience := range []string{"identity", "profile"} {
		codec, err := mcp.NewEnvelopeCodec(c.MCPContextSecret, audience)
		if err != nil {
			return nil, err
		}
		b.codecs[audience] = codec
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := b.rpc(ctx, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "sts-account", "version": "0.1.0"}}, &init); err != nil {
		return nil, err
	}
	if init.ProtocolVersion != "2025-06-18" && init.ProtocolVersion != "2025-11-25" {
		return nil, errors.New("unsupported MCP protocol")
	}
	b.protocol = init.ProtocolVersion
	if err := b.rpc(ctx, "notifications/initialized", nil, nil); err != nil {
		return nil, err
	}
	var listing struct {
		Tools []mcp.Tool `json:"tools"`
	}
	if err := b.rpc(ctx, "tools/list", map[string]any{}, &listing); err != nil {
		return nil, err
	}
	for _, remote := range listing.Tools {
		for original, policy := range policies {
			normalized := strings.ReplaceAll(original, ".", "_")
			if remote.Name != original && !strings.HasSuffix(strings.NewReplacer(".", "_", "-", "_").Replace(remote.Name), "_"+normalized) {
				continue
			}
			if _, exists := b.routes[original]; exists {
				return nil, errors.New("ambiguous MCP tool")
			}
			var wrapper struct {
				Properties struct {
					Payload json.RawMessage `json:"payload"`
				} `json:"properties"`
			}
			if json.Unmarshal(remote.InputSchema, &wrapper) != nil || len(wrapper.Properties.Payload) == 0 {
				return nil, errors.New("unsigned MCP tool")
			}
			b.routes[original] = remote.Name
			b.tools = append(b.tools, mcp.Tool{Name: original, OriginalName: original, HostPolicy: mcp.Policy(policy.Policy), Description: descriptions[original], InputSchema: wrapper.Properties.Payload})
		}
	}
	if len(b.routes) != len(policies) {
		names := []string{}
		for _, t := range listing.Tools {
			names = append(names, t.Name)
		}
		return nil, fmt.Errorf("account tool catalog incomplete: %v", names)
	}
	return b, nil
}
func (b *MCP) rpc(ctx context.Context, method string, params any, target any) error {
	id := b.sequence.Add(1)
	body := map[string]any{"jsonrpc": "2.0", "method": method}
	if target != nil {
		body["id"] = id
	}
	if params != nil {
		body["params"] = params
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("x-litellm-api-key", "Bearer "+b.key)
	req.Header.Set("MCP-Protocol-Version", b.protocol)
	if b.session != "" {
		req.Header.Set("Mcp-Session-Id", b.session)
	}
	response, err := b.client.Do(req)
	if err != nil {
		return errors.New("MCP unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("MCP request rejected (%d)", response.StatusCode)
	}
	if session := response.Header.Get("Mcp-Session-Id"); session != "" && method == "initialize" {
		b.session = session
	}
	if target == nil {
		return nil
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, 262145))
	if err != nil || len(data) > 262144 {
		return errors.New("MCP response exceeds limit")
	}
	var result struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 4096), 262144)
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				var candidate struct {
					ID uint64 `json:"id"`
				}
				line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if json.Unmarshal([]byte(line), &candidate) == nil && candidate.ID == id {
					data = []byte(line)
					found = true
					break
				}
			}
		}
		if !found {
			return errors.New("missing MCP response")
		}
	}
	if json.Unmarshal(data, &result) != nil || result.ID != id {
		return errors.New("MCP protocol error")
	}
	if len(result.Error) > 0 {
		if bytes.Contains(result.Error, []byte("account_policy_blocked")) {
			return errors.New("account_policy_blocked")
		}
		return errors.New("MCP protocol error")
	}
	if len(result.Result) == 0 {
		return errors.New("MCP protocol error")
	}
	return json.Unmarshal(result.Result, target)
}
func (b *MCP) Tools(context.Context) ([]mcp.Tool, error) { return b.tools, nil }
func (b *MCP) Call(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) (mcp.Result, error) {
	remote, ok := b.routes[name]
	if !ok {
		return mcp.Result{}, errors.New("tool not allowed")
	}
	if strings.HasPrefix(name, "profile.") && b.identity.SessionRef == "" {
		return mcp.Failure("authentication_required"), nil
	}
	audience, _, _ := strings.Cut(name, ".")
	data, err := b.codecs[audience].WrapV2(name, args, key, confirmed, b.identity)
	if err != nil {
		return mcp.Result{}, err
	}
	var envelope any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		return mcp.Result{}, errors.New("invalid envelope")
	}
	var result mcp.Result
	err = b.rpc(ctx, "tools/call", map[string]any{"name": remote, "arguments": envelope}, &result)
	return result, err
}

// Probe both directions through the real MCP endpoint. A denial from the Spring
// server alone is not proof that LiteLLM has registered its guardrails.
func (b *MCP) VerifyGuardrails(ctx context.Context) error {
	for _, key := range []string{"guardrail-probe-input", "guardrail-probe-output"} {
		raw, err := b.codecs["identity"].WrapV2("identity.login_status", json.RawMessage(`{}`), key, false, b.identity)
		if err != nil {
			return err
		}
		var envelope map[string]any
		if json.Unmarshal(raw, &envelope) != nil {
			return errors.New("invalid probe")
		}
		// The host_context schema deliberately leaves verification to the signed
		// context validator. This canary reaches the pre hook before schema errors.
		if key == "guardrail-probe-input" {
			envelope["host_context"].(map[string]any)["password"] = "sts-public-canary-not-a-secret"
		}
		var result mcp.Result
		err = b.rpc(ctx, "tools/call", map[string]any{"name": b.routes["identity.login_status"], "arguments": envelope}, &result)
		blocked := err != nil && err.Error() == "account_policy_blocked"
		if result.IsError {
			for _, c := range result.Content {
				blocked = blocked || strings.Contains(c.Text, "account_policy_blocked")
			}
		}
		if !blocked {
			return errors.New("required account guardrail did not block " + key)
		}
	}
	return nil
}
