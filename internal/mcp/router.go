package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"time"
)

type Policy string

const (
	ReadOnly       Policy = "read_only"
	ExplicitIntent Policy = "explicit_intent"
	ConfirmLater   Policy = "confirm_later"
)

type ServerConfig struct {
	Alias        string            `json:"alias"`
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	AllowedTools []string          `json:"allowed_tools"`
	Policies     map[string]Policy `json:"policies"`
}

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,23}$`)

func ValidateServers(servers []ServerConfig) error {
	if len(servers) > 8 {
		return errors.New("at most eight MCP servers")
	}
	aliases := map[string]bool{}
	for _, s := range servers {
		if !aliasPattern.MatchString(s.Alias) || aliases[s.Alias] || !filepath.IsAbs(s.Command) || len(s.AllowedTools) == 0 {
			return errors.New("invalid MCP server configuration")
		}
		aliases[s.Alias] = true
		seen := map[string]bool{}
		for _, n := range s.AllowedTools {
			p := s.Policies[n]
			if n == "" || seen[n] || (p != ReadOnly && p != ExplicitIntent && p != ConfirmLater) {
				return errors.New("each allowed tool requires an explicit host policy")
			}
			switch n {
			case "notes.delete", "agenda.cancel_event":
				if p != ConfirmLater {
					return errors.New("destructive built-in tool requires confirm_later")
				}
			case "notes.create", "agenda.create_event":
				if p != ExplicitIntent {
					return errors.New("creation built-in tool requires explicit_intent")
				}
			case "notes.list", "agenda.list_slots", "agenda.list_events":
				if p != ReadOnly {
					return errors.New("query built-in tool requires read_only")
				}
			}
			seen[n] = true
		}
		for n := range s.Policies {
			if !seen[n] {
				return errors.New("policy outside allowlist")
			}
		}
	}
	return nil
}

type route struct {
	client   *Client
	original string
}

// Router freezes discovery for this logical session. Failed processes are never
// restarted automatically: a mutation may already have committed before EOF.
type Router struct {
	tools   []Tool
	routes  map[string]route
	clients []*Client
}

func StartServers(ctx context.Context, configs []ServerConfig, timeout time.Duration) (*Router, error) {
	if err := ValidateServers(configs); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, errors.New("MCP timeout must be positive")
	}
	r := &Router{routes: map[string]route{}}
	for _, cfg := range configs {
		// Startup failure is isolated. Configured unavailable tools get bounded
		// placeholders so calls fail explicitly; healthy specs remain advertised.
		startCtx, stop := context.WithTimeout(ctx, timeout)
		// Lifetime belongs to ctx, initialization/discovery use their own deadline.
		client, err := StartWithTimeout(ctx, cfg.Command, cfg.Args, timeout)
		var discovered []Tool
		if err == nil {
			discovered, err = client.Tools(startCtx)
		}
		stop()
		if err != nil {
			discovered = nil
			if client != nil {
				client.Close()
			}
			client = nil
			for _, n := range cfg.AllowedTools {
				tool := Tool{Name: n, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}
				for _, known := range append(NotesTools(), AgendaTools()...) {
					if known.Name == n {
						tool = known
						break
					}
				}
				tool.Description = "Servidor indisponível. Não invente resultados; informe o erro. " + tool.Description
				discovered = append(discovered, tool)
			}
		} else {
			r.clients = append(r.clients, client)
		}
		permit := map[string]bool{}
		for _, n := range cfg.AllowedTools {
			permit[n] = true
		}
		seen := map[string]bool{}
		for _, t := range discovered {
			if !permit[t.Name] {
				continue
			}
			if seen[t.Name] {
				r.Close()
				return nil, errors.New("duplicate MCP tool")
			}
			seen[t.Name] = true
			original := t.Name
			t.Name = cfg.Alias + "." + original
			t.Server = cfg.Alias
			t.OriginalName = original
			t.HostPolicy = cfg.Policies[original]
			if _, ok := r.routes[t.Name]; ok {
				r.Close()
				return nil, errors.New("duplicate route")
			}
			r.routes[t.Name] = route{client, original}
			r.tools = append(r.tools, t)
		}
	}
	if len(r.tools) > 32 {
		r.Close()
		return nil, errors.New("union exceeds 32 tools")
	}
	return r, nil
}
func (r *Router) Tools(context.Context) ([]Tool, error) { return r.tools, nil }
func (r *Router) AllowedTools() []string {
	names := []string{}
	for _, t := range r.tools {
		names = append(names, t.Name)
	}
	return names
}
func (r *Router) Call(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) (Result, error) {
	b, ok := r.routes[name]
	if !ok || b.client == nil {
		return Result{}, errors.New("MCP unavailable")
	}
	return b.client.Call(ctx, b.original, args, key, confirmed)
}
func (r *Router) Close() {
	for _, c := range r.clients {
		c.Close()
	}
}
