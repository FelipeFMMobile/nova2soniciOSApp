package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	. "stsmodel.local/mcpwire"
	agentconfig "stsmodel.local/personal-assistant-agent/config"
	"stsmodel.local/personal-assistant-agent/config/integrations"
	"time"
)

type ServerConfig = agentconfig.ServerConfig

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
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
		if s.Cwd != "" && !filepath.IsAbs(s.Cwd) {
			return errors.New("MCP cwd must be absolute")
		}
		for target, source := range s.Env {
			if !envName.MatchString(target) || !envName.MatchString(source) {
				return errors.New("invalid MCP environment reference")
			}
			if _, ok := os.LookupEnv(source); !ok {
				return errors.New("missing MCP environment reference")
			}
		}
		seen := map[string]bool{}
		for _, n := range s.AllowedTools {
			p := s.Policies[n]
			if n == "" || seen[n] || (p != ReadOnly && p != ExplicitIntent && p != ConfirmLater) {
				return errors.New("each allowed tool requires an explicit host policy")
			}
			if err := validateBuiltInPolicy(n, p); err != nil {
				return err
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

func validateBuiltInPolicy(name string, policy Policy) error {
	return integrations.ValidatePolicy(name, string(policy))
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
		env := os.Environ()
		for target, source := range cfg.Env {
			env = append(env, target+"="+os.Getenv(source))
		}
		client, err := StartProcess(ctx, cfg.Command, cfg.Args, cfg.Cwd, env, timeout)
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
				tool := Tool{Name: n, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)}

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
