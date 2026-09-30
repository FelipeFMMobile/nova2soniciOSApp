// Package testkit exercises application binaries through their public protocols.
// It is only imported by tests; no business rules or database access live here.
package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	mcp "stsmodel.local/mcpwire"
	"testing"
	"time"
)

func Root() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(f), "../../.."))
}
func Binary(name string) string {
	dir := os.Getenv("STS_TEST_BIN_DIR")
	if dir == "" {
		dir = filepath.Join(Root(), "bin")
	}
	return filepath.Join(dir, name)
}
func Tools(app string) []mcp.Tool {
	data, err := os.ReadFile(filepath.Join(Root(), "apps", app, "contracts/v1/tools.json"))
	if err != nil {
		panic(err)
	}
	var ts []mcp.Tool
	if err = json.Unmarshal(data, &ts); err != nil {
		panic(err)
	}
	for i := range ts {
		var compact bytes.Buffer
		if err := json.Compact(&compact, ts[i].InputSchema); err != nil {
			panic(err)
		}
		ts[i].InputSchema = compact.Bytes()
	}
	return ts
}
func NotesTools() []mcp.Tool  { return Tools("mcp-notes") }
func AgendaTools() []mcp.Tool { return Tools("mcp-agenda") }

// Wire DTOs used by consumers; not imports of domain entities.
type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
type AgendaEvent struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Start      string `json:"start"`
	End        string `json:"end"`
	StartLocal string `json:"start_local"`
	EndLocal   string `json:"end_local"`
	Status     string `json:"status"`
}
type AgendaInput struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Duration int    `json:"duration_minutes"`
}
type Store struct{ client *mcp.Client }
type Agenda = Store

func open(app, path string) (*Store, error) {
	c, err := mcp.Start(context.Background(), Binary(app), []string{"-db", path})
	if err != nil {
		return nil, err
	}
	return &Store{c}, nil
}
func Open(path string) (*Store, error)        { return open("mcp-notes", path) }
func OpenAgenda(path string) (*Agenda, error) { return open("mcp-agenda", path) }
func (s *Store) Close() error                 { s.client.Close(); return nil }
func (s *Store) call(ctx context.Context, name string, args any, key string, confirmed bool) (json.RawMessage, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	r, err := s.client.Call(ctx, name, data, key, confirmed)
	if err != nil {
		return nil, err
	}
	if r.IsError || len(r.Content) == 0 {
		return nil, errors.New("MCP call failed")
	}
	return json.RawMessage(r.Content[0].Text), nil
}
func (s *Store) Mutate(ctx context.Context, key, name string, args json.RawMessage) (json.RawMessage, error) {
	return s.call(ctx, name, args, key, true)
}
func (s *Store) List(ctx context.Context, q string) ([]Note, error) {
	data, err := s.call(ctx, "notes.list", map[string]string{"query": q}, "", false)
	if err != nil {
		return nil, err
	}
	var v struct {
		Notes []Note `json:"notes"`
	}
	err = json.Unmarshal(data, &v)
	return v.Notes, err
}
func (s *Store) Events(ctx context.Context, a, b string) ([]AgendaEvent, error) {
	data, err := s.call(ctx, "agenda.list_events", map[string]string{"start": a, "end": b}, "", false)
	if err != nil {
		return nil, err
	}
	var v struct {
		Events []AgendaEvent `json:"events"`
	}
	err = json.Unmarshal(data, &v)
	return v.Events, err
}
func Serve(ctx context.Context, in io.Reader, out io.Writer, s *Store) error {
	return serve(ctx, in, out, s)
}
func ServeAgenda(ctx context.Context, in io.Reader, out io.Writer, s *Store) error {
	return serve(ctx, in, out, s)
}
func serve(ctx context.Context, in io.Reader, out io.Writer, s *Store) error {
	tools, err := s.client.Tools(ctx)
	if err != nil {
		return err
	}
	return mcp.ServeTools(ctx, in, out, "test-proxy", tools, func(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) mcp.Result {
		r, err := s.client.Call(ctx, name, args, key, confirmed)
		if err != nil {
			return mcp.Failure("test_proxy_failed")
		}
		return r
	})
}

type Gateway struct {
	URL string
	cmd *exec.Cmd
}

func (g *Gateway) Close() { _ = g.cmd.Process.Kill(); _ = g.cmd.Wait() }
func StartGateway(t *testing.T) *Gateway {
	t.Helper()
	address := AvailableAddress(t)
	agentEnv := append(os.Environ(), "STS_AGENT_ADDRESS="+address, "STS_AGENT_TOKEN=test-internal-token")
	StartService(t, "personal-assistant-agent", agentEnv, address)
	catalog := `[{"id":"personal-assistant","endpoint":"ws://` + address + `/v1/session","protocol":"agent-session/v1","token_env":"STS_AGENT_TOKEN"}]`
	env := []string{"STS_AGENT_TOKEN=test-internal-token", "STS_AGENT_CATALOG=" + catalog}
	for _, key := range []string{"STS_PROVIDER", "STS_DEVELOPMENT_TOKEN"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return StartGatewayWithEnv(t, env)
}
func AvailableAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	return address
}
func StartGatewayWithEnv(t *testing.T, extra []string) *Gateway {
	t.Helper()
	address := AvailableAddress(t)
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !strings.HasPrefix(key, "STS_") && !strings.HasPrefix(key, "NOVA_") && !strings.HasPrefix(key, "AWS_") {
			env = append(env, item)
		}
	}
	env = append(env, extra...)
	env = append(env, "STS_GATEWAY_ADDRESS="+address)
	return StartService(t, "gateway", env, address)
}
func StartService(t *testing.T, name string, env []string, address string) *Gateway {
	t.Helper()
	cmd := exec.Command(Binary(name))
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{"http://" + address, cmd}
	t.Cleanup(g.Close)
	client := http.Client{Timeout: time.Second}
	for i := 0; i < 150; i++ {
		r, e := client.Get(g.URL + "/healthz")
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return g
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not start at %s", name, address)
	return nil
}
