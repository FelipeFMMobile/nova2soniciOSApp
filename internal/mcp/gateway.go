package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GatewayServerConfig struct {
	Alias        string            `json:"alias"`
	ServerID     string            `json:"server_id"`
	AllowedTools []string          `json:"allowed_tools"`
	Policies     map[string]Policy `json:"policies"`
}

type gatewayRoute struct {
	serverID, original string
	codec              *EnvelopeCodec
}
type Gateway struct {
	endpoint, apiKey string
	client           *http.Client
	tools            []Tool
	routes           map[string]gatewayRoute
}

func ValidateGatewayServers(servers []GatewayServerConfig) error {
	if len(servers) == 0 || len(servers) > 8 {
		return errors.New("LiteLLM MCP gateway requires one to eight servers")
	}
	aliases, ids := map[string]bool{}, map[string]bool{}
	for _, server := range servers {
		if !aliasPattern.MatchString(server.Alias) || server.ServerID == "" || aliases[server.Alias] || ids[server.ServerID] || len(server.AllowedTools) == 0 {
			return errors.New("invalid LiteLLM MCP server configuration")
		}
		aliases[server.Alias], ids[server.ServerID] = true, true
		seen := map[string]bool{}
		for _, name := range server.AllowedTools {
			policy := server.Policies[name]
			if name == "" || seen[name] || (policy != ReadOnly && policy != ExplicitIntent && policy != ConfirmLater) {
				return errors.New("each gateway tool requires an explicit host policy")
			}
			if err := validateBuiltInPolicy(name, policy); err != nil {
				return err
			}
			seen[name] = true
		}
		if len(seen) != len(server.Policies) {
			return errors.New("gateway policy outside allowlist")
		}
	}
	return nil
}

func NewGateway(ctx context.Context, endpoint, apiKey, secret string, servers []GatewayServerConfig, timeout time.Duration) (*Gateway, error) {
	return newGateway(ctx, endpoint, apiKey, secret, servers, &http.Client{Timeout: timeout}, timeout)
}

func newGateway(ctx context.Context, endpoint, apiKey, secret string, servers []GatewayServerConfig, client *http.Client, timeout time.Duration) (*Gateway, error) {
	if err := ValidateGatewayServers(servers); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid LiteLLM URL")
	}
	if apiKey == "" || timeout <= 0 {
		return nil, errors.New("LiteLLM API key and positive timeout required")
	}
	g := &Gateway{endpoint: strings.TrimRight(endpoint, "/"), apiKey: apiKey, client: client, routes: map[string]gatewayRoute{}}
	for _, server := range servers {
		codec, err := NewEnvelopeCodec(secret, server.Alias)
		if err != nil {
			return nil, err
		}
		var listed struct {
			Tools []struct {
				Name, Description string
				InputSchema       json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
			Error any `json:"error"`
		}
		if err = g.request(ctx, http.MethodGet, "/mcp-rest/tools/list?server_id="+url.QueryEscape(server.ServerID), nil, &listed); err != nil {
			return nil, err
		}
		allowed := map[string]Policy{}
		for _, name := range server.AllowedTools {
			allowed[name] = server.Policies[name]
		}
		for _, remote := range listed.Tools {
			policy, ok := allowed[remote.Name]
			if !ok {
				continue
			}
			var schema struct {
				Properties struct {
					Payload json.RawMessage `json:"payload"`
				} `json:"properties"`
			}
			if json.Unmarshal(remote.InputSchema, &schema) != nil || len(schema.Properties.Payload) == 0 {
				return nil, errors.New("MCP server does not expose the signed STS envelope")
			}
			name := server.Alias + "." + remote.Name
			if _, exists := g.routes[name]; exists {
				return nil, errors.New("duplicate gateway route")
			}
			g.routes[name] = gatewayRoute{server.ServerID, remote.Name, codec}
			g.tools = append(g.tools, Tool{Server: server.Alias, OriginalName: remote.Name, HostPolicy: policy, Name: name, Description: remote.Description, InputSchema: schema.Properties.Payload})
			delete(allowed, remote.Name)
		}
		if len(allowed) != 0 {
			return nil, fmt.Errorf("LiteLLM did not advertise all allowed tools for %s", server.Alias)
		}
	}
	if len(g.tools) > 32 {
		return nil, errors.New("union exceeds 32 tools")
	}
	return g, nil
}

func (g *Gateway) request(ctx context.Context, method, path string, body any, target any) error {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, g.endpoint+path, input)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+g.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return errors.New("LiteLLM MCP request failed")
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, MaxMessage+1)
	data, err := io.ReadAll(limited)
	if err != nil || len(data) > MaxMessage {
		return errors.New("invalid LiteLLM MCP response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("LiteLLM MCP returned HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return errors.New("invalid LiteLLM MCP JSON")
	}
	return nil
}

func (g *Gateway) Tools(context.Context) ([]Tool, error) { return append([]Tool(nil), g.tools...), nil }
func (g *Gateway) AllowedTools() []string {
	names := make([]string, len(g.tools))
	for i := range g.tools {
		names[i] = g.tools[i].Name
	}
	return names
}
func (g *Gateway) Call(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) (Result, error) {
	route, ok := g.routes[name]
	if !ok {
		return Result{}, errors.New("MCP route unavailable")
	}
	envelope, err := route.codec.Wrap(route.original, args, key, confirmed)
	if err != nil {
		return Result{}, err
	}
	var arguments map[string]any
	if json.Unmarshal(envelope, &arguments) != nil {
		return Result{}, errors.New("invalid signed envelope")
	}
	var result Result
	err = g.request(ctx, http.MethodPost, "/mcp-rest/tools/call", map[string]any{"server_id": route.serverID, "name": route.original, "arguments": arguments}, &result)
	return result, err
}
