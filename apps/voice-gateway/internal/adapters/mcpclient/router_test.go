package mcpclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	. "stsmodel.local/mcpwire"
	storage "stsmodel.local/testkit"
	"testing"
	"time"
)

func TestRouterProcess(t *testing.T) {
	if os.Getenv("STS_ROUTER_HELPER") != "1" {
		return
	}
	kind, path := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	if kind == "notes" {
		s, err := storage.Open(path)
		if err != nil {
			os.Exit(2)
		}
		defer s.Close()
		_ = storage.Serve(context.Background(), os.Stdin, os.Stdout, s)
	} else {
		s, err := storage.OpenAgenda(path)
		if err != nil {
			os.Exit(2)
		}
		defer s.Close()
		if kind == "hang" {
			_ = ServeTools(context.Background(), os.Stdin, os.Stdout, "hang", storage.AgendaTools(), func(context.Context, string, json.RawMessage, string, bool) Result {
				time.Sleep(time.Hour)
				return Failure("never")
			})
		} else {
			_ = storage.ServeAgenda(context.Background(), os.Stdin, os.Stdout, s)
		}
	}
	os.Exit(0)
}
func routerConfigs(t *testing.T, kind string) []ServerConfig {
	t.Helper()
	t.Setenv("STS_ROUTER_HELPER", "1")
	dir := t.TempDir()
	configs := []ServerConfig{
		{Alias: "memo", Command: os.Args[0], Args: []string{"-test.run=^TestRouterProcess$", "--", "notes", filepath.Join(dir, "notes.sqlite")}, AllowedTools: []string{"notes.list", "notes.create"}, Policies: map[string]Policy{"notes.list": ReadOnly, "notes.create": ExplicitIntent}},
		{Alias: "local", Command: os.Args[0], Args: []string{"-test.run=^TestRouterProcess$", "--", kind, filepath.Join(dir, "agenda.sqlite")}, AllowedTools: []string{"agenda.list_events", "agenda.create_event"}, Policies: map[string]Policy{"agenda.list_events": ReadOnly, "agenda.create_event": ExplicitIntent}},
	}
	configs[0].Command = storage.Binary("mcp-notes")
	configs[0].Args = []string{"-db", filepath.Join(dir, "notes.sqlite")}
	if kind != "hang" {
		configs[1].Command = storage.Binary("mcp-agenda")
		configs[1].Args = []string{"-db", filepath.Join(dir, "agenda.sqlite")}
	}
	return configs

}
func TestTwoStdioServersAndIsolatedTimeout(t *testing.T) {
	for _, kind := range []string{"agenda", "hang", "missing"} {
		t.Run(kind, func(t *testing.T) {
			cfg := routerConfigs(t, kind)
			if kind == "missing" {
				cfg[1].Command = "/nonexistent/sts-mcp"
			}
			r, err := StartServers(context.Background(), cfg, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			tools, err := r.Tools(context.Background())
			if err != nil || len(tools) != 4 {
				t.Fatal(tools, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			result, err := r.Call(ctx, "local.agenda.list_events", json.RawMessage(`{"start":"2030-05-20T00:00:00Z","end":"2030-05-21T00:00:00Z"}`), "read", false)
			if kind == "agenda" {
				if err != nil || result.IsError {
					t.Fatal(result, err)
				}
			} else if err == nil {
				t.Fatal("unavailable succeeded")
			}
			result, err = r.Call(context.Background(), "memo.notes.create", json.RawMessage(`{"title":"healthy","content":"fictitious"}`), "memo/request/create", false)
			if err != nil || result.IsError {
				t.Fatal("healthy server lost", result, err)
			}
			result, err = r.Call(context.Background(), "memo.notes.list", json.RawMessage(`{}`), "read", false)
			if err != nil || result.IsError {
				t.Fatal(result, err)
			}
		})
	}
}
func TestServerConfigurationPolicies(t *testing.T) {
	valid := ServerConfig{Alias: "a", Command: "/bin/server", AllowedTools: []string{"notes.list"}, Policies: map[string]Policy{"notes.list": ReadOnly}}
	for _, change := range []func(*ServerConfig){func(s *ServerConfig) { s.Command = "relative" }, func(s *ServerConfig) { s.Alias = "a.b" }, func(s *ServerConfig) { s.Policies = nil }, func(s *ServerConfig) { s.AllowedTools = []string{"x", "x"} }} {
		s := valid
		change(&s)
		if ValidateServers([]ServerConfig{s}) == nil {
			t.Fatal("bad config")
		}
	}
	if ValidateServers([]ServerConfig{valid, valid}) == nil {
		t.Fatal("duplicate aliases")
	}
}

func TestProcessEnvironmentReferences(t *testing.T) {
	cfg := ServerConfig{Alias: "memo", Command: "/bin/unused", AllowedTools: []string{"notes.list"}, Policies: map[string]Policy{"notes.list": ReadOnly}, Cwd: t.TempDir(), Env: map[string]string{"API_KEY": "STS_TEST_SOURCE_KEY"}}
	t.Setenv("STS_TEST_SOURCE_KEY", "test-only")
	if err := ValidateServers([]ServerConfig{cfg}); err != nil {
		t.Fatal(err)
	}
	cfg.Env["API_KEY"] = "UNSET_STS_TEST_SOURCE_KEY"
	t.Setenv("UNSET_STS_TEST_SOURCE_KEY", "")
	os.Unsetenv("UNSET_STS_TEST_SOURCE_KEY")
	if ValidateServers([]ServerConfig{cfg}) == nil {
		t.Fatal("missing env accepted")
	}
	cfg.Env = nil
	cfg.Cwd = "relative"
	if ValidateServers([]ServerConfig{cfg}) == nil {
		t.Fatal("relative cwd accepted")
	}
}
