package gateway

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/storage"
	"sync/atomic"
	"testing"
	"time"
)

func TestGatewayAgendaHelper(t *testing.T) {
	if os.Getenv("STS_GATEWAY_MCP_HELPER") != "1" {
		return
	}
	s, err := storage.OpenAgenda(os.Getenv("STS_GATEWAY_AGENDA_DB"))
	if err != nil {
		os.Exit(2)
	}
	defer s.Close()
	_ = mcp.ServeAgenda(context.Background(), os.Stdin, os.Stdout, s)
	os.Exit(0)
}
func multiConfig(t *testing.T) (string, string, []mcp.ServerConfig) {
	t.Helper()
	dir := t.TempDir()
	notes, agenda := filepath.Join(dir, "notes.sqlite"), filepath.Join(dir, "agenda.sqlite")
	t.Setenv("STS_GATEWAY_MCP_HELPER", "1")
	t.Setenv("STS_GATEWAY_MCP_DB", notes)
	t.Setenv("STS_GATEWAY_AGENDA_DB", agenda)
	cfg := []mcp.ServerConfig{
		{Alias: "memo", Command: os.Args[0], Args: []string{"-test.run=^TestGatewayMCPHelper$"}, AllowedTools: []string{"notes.list", "notes.create", "notes.delete"}, Policies: map[string]mcp.Policy{"notes.list": mcp.ReadOnly, "notes.create": mcp.ExplicitIntent, "notes.delete": mcp.ConfirmLater}},
		{Alias: "local", Command: os.Args[0], Args: []string{"-test.run=^TestGatewayAgendaHelper$"}, AllowedTools: []string{"agenda.list_slots", "agenda.list_events", "agenda.create_event", "agenda.cancel_event"}, Policies: map[string]mcp.Policy{"agenda.list_slots": mcp.ReadOnly, "agenda.list_events": mcp.ReadOnly, "agenda.create_event": mcp.ExplicitIntent, "agenda.cancel_event": mcp.ConfirmLater}},
	}
	return notes, agenda, cfg
}

const eventArgs = `{"title":"Consulta fictícia","start":"2030-05-20T09:00:00-03:00","end":"2030-05-20T10:00:00-03:00"}`
const rangeArgs = `{"start":"2030-05-20T09:00:00-03:00","end":"2030-05-20T18:00:00-03:00"}`

func bridgeResult(t *testing.T, c *websocket.Conn) mcp.Result {
	t.Helper()
	value := waitTool(c)
	if value == nil {
		t.Error("missing correlated tool result")
		return mcp.Failure("missing")
	}
	data, _ := value["content"].(string)
	var r mcp.Result
	if json.Unmarshal([]byte(data), &r) != nil {
		t.Error("bad result", value)
	}
	return r
}
func finishMock(c *websocket.Conn, id string) {
	nativeAudio(c, id)
	_ = rawEvent(c, "contentEnd", map[string]any{"contentId": id, "stopReason": "END_TURN"})
}

// Mock scripts choose calls explicitly. This proves host routing/data flow,
// never Nova's ability to choose tools or interpret natural language.
func TestTwoRealMCPsMockNovaScenarios(t *testing.T) {
	scenarios := []string{"notes_only", "agenda_only", "note_to_agenda", "agenda_to_note", "ambiguous", "ordinary", "cancel_approved", "cancel_refused", "invalid_arguments", "retry", "spoken_create"}
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			notesPath, agendaPath, servers := multiConfig(t)
			s, err := storage.Open(notesPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Mutate(context.Background(), "fixture", "notes.create", json.RawMessage(`{"title":"Consulta fictícia","content":"2030-05-20T09:00:00-03:00"}`))
			s.Close()
			if err != nil {
				t.Fatal(err)
			}
			url := bridge(t, func(c *websocket.Conn, start map[string]any) {
				tools, ok := start["tools"].([]any)
				if !ok || len(tools) != 7 {
					t.Error("union missing")
					return
				}
				for _, tool := range tools {
					spec := tool.(map[string]any)["toolSpec"].(map[string]any)
					if _, ok := spec["inputSchema"].(map[string]any)["json"].(string); !ok {
						t.Error("wire schema")
					}
				}
				var input map[string]any
				if c.ReadJSON(&input) != nil {
					return
				}
				phrase := "Agende Consulta fictícia em 2030-05-20 às 09:00 por uma hora"
				switch scenario {
				case "spoken_create":
					phrase = "Agende reunião dia 18 de setembro de 2026 às 10 horas por uma hora"
				case "ordinary":
					phrase = "Olá tudo bem"
				case "note_to_agenda":
					phrase = "Consulte a nota Consulta fictícia e agende usando seu conteúdo em 2030-05-20 às 09:00 por uma hora"
				case "notes_only":
					phrase = "Consulte a nota Consulta fictícia"
				case "agenda_only":
					phrase = "Consulte horários de 20 de maio de 2030"
				case "agenda_to_note":
					phrase = "Consulte a agenda e registre o resultado em notas"
				}
				userPhrase(c, "u1", phrase)
				call := func(id, name, args string) mcp.Result {
					nativeTool(c, id, name, args)
					return bridgeResult(t, c)
				}
				switch scenario {
				case "spoken_create":
					r := call("spoken", "local_agenda_create_event", `{"title":"reunião","start":"2026-09-18T10:00:00-03:00","end":"2026-09-18T11:00:00-03:00"}`)
					if r.IsError || mcp.Status(r) != "created" {
						t.Error(r)
					}
				case "notes_only":
					if r := call("n1", "memo_notes_list", `{"query":"Consulta"}`); r.IsError || !strings.Contains(r.Content[0].Text, "Consulta fictícia") {
						t.Error(r)
					}
				case "agenda_only":
					if r := call("a1", "local_agenda_list_slots", strings.TrimSuffix(rangeArgs, "}")+`,"duration_minutes":60}`); r.IsError {
						t.Error(r)
					}
				case "note_to_agenda":
					r := call("n1", "memo_notes_list", `{"query":"Consulta"}`)
					var body struct {
						Notes []storage.Note `json:"notes"`
					}
					_ = json.Unmarshal([]byte(r.Content[0].Text), &body)
					if len(body.Notes) != 1 {
						t.Error(r)
						return
					}
					args, _ := json.Marshal(map[string]string{"title": body.Notes[0].Title, "start": body.Notes[0].Content, "end": "2030-05-20T10:00:00-03:00"})
					if r = call("a1", "local_agenda_create_event", string(args)); r.IsError {
						t.Error(r)
					}
				case "agenda_to_note":
					r := call("a1", "local_agenda_list_events", rangeArgs)
					args, _ := json.Marshal(map[string]string{"title": "Agenda consultada", "content": r.Content[0].Text})
					if r = call("n1", "memo_notes_create", string(args)); r.IsError {
						t.Error(r)
					}
				case "ambiguous":
					finishMock(c, "pre")
					userPhrase(c, "u2", "talvez marque algo amanhã")
					if r := call("a1", "local_agenda_create_event", eventArgs); r.IsError || mcp.Status(r) != "created" {
						t.Error(r)
					}
				case "ordinary": // no scripted toolUse: verifies gateway doesn't invent calls
				case "invalid_arguments":
					if r := call("a1", "local_agenda_create_event", `{"title":"x"}`); !r.IsError {
						t.Error(r)
					}
				case "retry":
					r1 := call("a1", "local_agenda_create_event", eventArgs)
					r2 := call("a2", "local_agenda_create_event", eventArgs)
					if r1.IsError || r2.IsError || r1.Content[0].Text != r2.Content[0].Text {
						t.Error(r1, r2)
					}
				case "cancel_approved", "cancel_refused":
					r := call("a1", "local_agenda_create_event", eventArgs)
					var body struct {
						Event storage.AgendaEvent `json:"event"`
					}
					_ = json.Unmarshal([]byte(r.Content[0].Text), &body)
					args, _ := json.Marshal(map[string]string{"id": body.Event.ID})
					r = call("c1", "local_agenda_cancel_event", string(args))
					if mcp.Status(r) != "confirmation_required" {
						t.Error(r)
					}
					finishMock(c, "first")
					phrase := "não"
					if scenario == "cancel_approved" {
						phrase = "confirmo cancelar agendamento"
					}
					userPhrase(c, "u2", phrase)
					if scenario == "cancel_approved" {
						if r = call("c2", "local_agenda_cancel_event", string(args)); r.IsError || mcp.Status(r) != "cancelled" {
							t.Error(r)
						}
					} else {
						if r = call("read", "local_agenda_list_events", rangeArgs); r.IsError || !strings.Contains(r.Content[0].Text, `"status":"active"`) {
							t.Error(r)
						}
					}
				}
				finishMock(c, "final")
				for c.ReadJSON(&input) == nil {
				}
			})
			cfg := testConfig()
			cfg.Provider = "nova"
			cfg.NovaBridgeURL = url
			cfg.MCPServers = servers
			cfg.DatabasePath = filepath.Join(t.TempDir(), "audit.sqlite")
			cfg.MCPEvidencePath = filepath.Join(t.TempDir(), "private.jsonl")
			if dir := os.Getenv("STS_TEST_EVIDENCE_DIR"); dir != "" {
				cfg.MCPEvidencePath = filepath.Join(dir, scenario+".jsonl")
			}
			_, _, ws := setup(t, cfg)
			c := dial(t, ws, "")
			sid := startNova(t, c)
			send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: "input-1", Sequence: 1, SampleRate: 16000, Audio: "AAAAAA=="})
			completions := 1
			if scenario == "ambiguous" || strings.HasPrefix(scenario, "cancel_") {
				completions = 2
			}
			for i := 0; i < completions; i++ {
				until(t, c, protocol.TurnCompleted)
			}
			send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
			until(t, c, protocol.SessionStopped)
			c.Close()
			a, err := storage.OpenAgenda(agendaPath)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			start, end := "2030-05-20T00:00:00Z", "2030-05-21T00:00:00Z"
			if scenario == "spoken_create" {
				start, end = "2026-09-18T00:00:00Z", "2026-09-19T00:00:00Z"
			}
			events, err := a.Events(context.Background(), start, end)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if scenario == "note_to_agenda" || scenario == "ambiguous" || scenario == "retry" || scenario == "spoken_create" || strings.HasPrefix(scenario, "cancel_") {
				want = 1
			}
			if len(events) != want {
				t.Fatal("effects", events)
			}
			if scenario == "cancel_approved" && events[0].Status != "cancelled" {
				t.Fatal(events)
			}
			if scenario == "cancel_refused" && events[0].Status != "active" {
				t.Fatal(events)
			}
			evidence, err := os.ReadFile(cfg.MCPEvidencePath)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "ordinary" {
				if len(evidence) != 0 {
					t.Fatal("ordinary invoked tool")
				}
			} else if !strings.Contains(string(evidence), `"server":`) {
				t.Fatal("no private evidence")
			}
			info, _ := os.Stat(cfg.MCPEvidencePath)
			if info.Mode().Perm() != 0600 {
				t.Fatal("private permissions")
			}
		})
	}
}

func TestMultiNovaRenewalKeepsRouting(t *testing.T) {
	_, _, servers := multiConfig(t)
	var connections atomic.Int64
	advertised := make(chan int, 4)
	url := bridge(t, func(c *websocket.Conn, start map[string]any) {
		n := connections.Add(1)
		tools, _ := start["tools"].([]any)
		advertised <- len(tools)
		var input map[string]any
		if c.ReadJSON(&input) != nil {
			return
		}
		nativeUser(c, "u")
		name := "memo_notes_list"
		args := `{}`
		if n > 1 {
			name = "local_agenda_list_events"
			args = rangeArgs
		}
		nativeTool(c, "read"+strconv.FormatInt(n, 10), name, args)
		if r := bridgeResult(t, c); r.IsError {
			t.Error(r)
		}
		finishMock(c, "answer")
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.NovaBridgeURL = url
	cfg.MCPServers = servers
	cfg.NovaMaxSessionAge = 300 * time.Millisecond
	_, _, ws := setup(t, cfg)
	c := dial(t, ws, "")
	sid := startNova(t, c)
	appendAudio(t, c, sid, "input-1", 1)
	until(t, c, protocol.TurnCompleted)
	until(t, c, protocol.SessionRenewed)
	appendAudio(t, c, sid, "input-2", 1)
	e := until(t, c, protocol.ToolResult)
	if e.Tool.Name != "local.agenda.list_events" {
		t.Fatal(e)
	}
	until(t, c, protocol.TurnCompleted)
	for i := 0; i < 2; i++ {
		if n := <-advertised; n != 7 {
			t.Fatal("renewal lost specs", n)
		}
	}
	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
	until(t, c, protocol.SessionStopped)
}

func TestGatewaySlowAgendaHelper(t *testing.T) {
	if os.Getenv("STS_GATEWAY_MCP_HELPER") != "1" {
		return
	}
	s, err := storage.OpenAgenda(os.Getenv("STS_GATEWAY_AGENDA_DB"))
	if err != nil {
		os.Exit(2)
	}
	defer s.Close()
	_ = mcp.ServeTools(context.Background(), os.Stdin, os.Stdout, "slow-agenda", mcp.AgendaTools(), func(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) mcp.Result {
		time.Sleep(250 * time.Millisecond)
		var i storage.AgendaInput
		_ = json.Unmarshal(args, &i)
		events, err := s.Events(ctx, i.Start, i.End)
		if err != nil {
			return mcp.Failure("storage_failed")
		}
		return mcp.TextResult(map[string]any{"status": "ok", "events": events}, false)
	})
	os.Exit(0)
}
func TestMultiMCPAudioAndTimeoutIsolation(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "audio", true: "timeout"}[timeout], func(t *testing.T) {
			_, _, servers := multiConfig(t)
			servers[1].Args = []string{"-test.run=^TestGatewaySlowAgendaHelper$"}
			forwarded := make(chan struct{}, 1)
			url := bridge(t, func(c *websocket.Conn, start map[string]any) {
				var input map[string]any
				if c.ReadJSON(&input) != nil {
					return
				}
				nativeUser(c, "u")
				nativeTool(c, "slow", "local_agenda_list_events", rangeArgs)
				if !timeout {
					for c.ReadJSON(&input) == nil {
						if input["type"] == "audio.append" {
							forwarded <- struct{}{}
							break
						}
					}
				}
				r := bridgeResult(t, c)
				if r.IsError != timeout {
					t.Error("timeout outcome", r)
				}
				nativeTool(c, "healthy", "memo_notes_list", `{}`)
				if r = bridgeResult(t, c); r.IsError {
					t.Error("healthy tool lost", r)
				}
				finishMock(c, "answer")
				for c.ReadJSON(&input) == nil {
				}
			})
			cfg := testConfig()
			cfg.Provider = "nova"
			cfg.NovaBridgeURL = url
			cfg.MCPServers = servers
			cfg.MCPTimeout = time.Second
			if timeout {
				cfg.MCPTimeout = 100 * time.Millisecond
			}
			_, _, ws := setup(t, cfg)
			c := dial(t, ws, "")
			sid := startNova(t, c)
			appendAudio(t, c, sid, "input", 1)
			until(t, c, protocol.ToolStarted)
			if !timeout {
				appendAudio(t, c, sid, "input", 2)
				select {
				case <-forwarded:
				case <-time.After(time.Second):
					t.Fatal("audio blocked by agenda")
				}
			}
			until(t, c, protocol.ToolResult)
			until(t, c, protocol.ToolResult)
			until(t, c, protocol.TurnCompleted)
			send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
			until(t, c, protocol.SessionStopped)
		})
	}
}

func TestAgendaGatewayDurableRetryAcrossSessions(t *testing.T) {
	_, agendaPath, servers := multiConfig(t)
	results := make(chan string, 2)
	url := bridge(t, func(c *websocket.Conn, start map[string]any) {
		var input any
		if c.ReadJSON(&input) != nil {
			return
		}
		userPhrase(c, "u", "Agende teste em 2030-05-20 às 09:00 por uma hora")
		nativeTool(c, "create", "local_agenda_create_event", eventArgs)
		r := bridgeResult(t, c)
		if r.IsError {
			t.Error(r)
			return
		}
		results <- r.Content[0].Text
		finishMock(c, "answer")
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.NovaBridgeURL = url
	cfg.MCPServers = servers
	_, _, ws := setup(t, cfg)
	for i := 0; i < 2; i++ {
		c := dial(t, ws, "")
		send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "nova", RequestID: "durable-agenda-request"})
		sid := until(t, c, protocol.SessionReady).SessionID
		appendAudio(t, c, sid, "input", 1)
		until(t, c, protocol.TurnCompleted)
		send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
		until(t, c, protocol.SessionStopped)
		c.Close()
	}
	if a, b := <-results, <-results; a != b {
		t.Fatal("durable result changed")
	}
	s, err := storage.OpenAgenda(agendaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	events, err := s.Events(context.Background(), "2030-05-20T00:00:00Z", "2030-05-21T00:00:00Z")
	if err != nil || len(events) != 1 {
		t.Fatal(events, err)
	}
}
