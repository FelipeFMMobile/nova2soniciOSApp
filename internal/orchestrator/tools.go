// Package orchestrator owns tool permission decisions, not the model or bridge.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/protocol"
)

type Backend interface {
	Tools(context.Context) ([]mcp.Tool, error)
	Call(context.Context, string, json.RawMessage, string, bool) (mcp.Result, error)
}
type binding struct {
	tool   mcp.Tool
	schema *jsonschema.Schema
}
type Pending struct {
	OperationID string
	Name        string
	Args        json.RawMessage
	TurnID      string
	Expires     time.Time
	Approved    bool
}
type Session struct {
	backend      Backend
	tools        map[string]binding
	Specs        []map[string]any
	namespace    string
	Pending      *Pending
	seen         map[string]string
	cache        map[string]mcp.Result
	calls        int
	mutationArgs map[string]string
}
type Job struct {
	ID, Name, Key, TurnID string
	Args                  json.RawMessage
	Confirmed             bool
}

func New(ctx context.Context, backend Backend, allowed []string, namespace string) (*Session, error) {
	discovered, err := backend.Tools(ctx)
	if err != nil {
		return nil, err
	}
	permit := map[string]bool{}
	for _, name := range allowed {
		permit[name] = true
	}
	s := &Session{backend: backend, tools: map[string]binding{}, namespace: namespace, seen: map[string]string{}, cache: map[string]mcp.Result{}, mutationArgs: map[string]string{}}
	for _, tool := range discovered {
		if !permit[tool.Name] {
			continue
		}
		name := strings.ReplaceAll(strings.ReplaceAll(tool.Name, ".", "_"), "-", "_")
		if !protocol.ValidID(name) || len(name) > 64 || len(tool.Description) > 4096 {
			return nil, errors.New("unsupported tool metadata")
		}
		if _, duplicate := s.tools[name]; duplicate {
			return nil, errors.New("tool name mapping collision")
		}
		schema, err := mcp.Compile(tool.InputSchema)
		if err != nil {
			return nil, errors.New("invalid MCP tool schema")
		}
		s.tools[name] = binding{tool: tool, schema: schema}
		// Explicit approval policy lives here, not in untrusted MCP annotations.
		description := tool.Description
		if tool.Name != "notes.list" && tool.Name != "notes.create" {
			description += " Esta ação exige confirmação do usuário em um turno posterior. Se receber confirmation_required, peça confirmo excluir (notas) ou confirmo executar, depois chame novamente com os mesmos argumentos."
		}
		// Bedrock's bidirectional wire format requires a JSON-encoded string,
		// unlike MCP's inputSchema object (and some simplified AWS examples).
		s.Specs = append(s.Specs, map[string]any{"toolSpec": map[string]any{"name": name, "description": description, "inputSchema": map[string]any{"json": string(tool.InputSchema)}}})
	}
	if len(s.Specs) == 0 {
		return nil, errors.New("no allowed MCP tools discovered")
	}
	return s, nil
}

func key(namespace, name string, args json.RawMessage) string {
	var value any
	_ = json.Unmarshal(args, &value)
	canonical, _ := json.Marshal(value)
	hash := sha256.Sum256([]byte(namespace + "\x00" + name + "\x00" + string(canonical)))
	return hex.EncodeToString(hash[:])
}

// Plan is called only by the gateway session owner. A nil immediate result
// means the returned job may run asynchronously; it must call Complete later.
func (s *Session) Plan(id, name, turn string, args json.RawMessage) (Job, *mcp.Result) {
	fail := func(code string) (Job, *mcp.Result) {
		r := mcp.Failure(code)
		return Job{ID: id, Name: name, TurnID: turn}, &r
	}
	if !protocol.ValidID(id) {
		return fail("invalid_tool_id")
	}
	b, ok := s.tools[name]
	if !ok {
		return fail("tool_not_allowed")
	}
	if mcp.Validate(b.schema, args) != nil {
		return fail("invalid_arguments")
	}
	k := key(s.namespace, b.tool.Name, args)
	if previous, ok := s.seen[id]; ok && previous != k {
		return fail("tool_id_conflict")
	}
	if s.calls >= 128 {
		return fail("tool_limit_reached")
	}
	s.calls++
	s.seen[id] = k
	j := Job{ID: id, Name: b.tool.Name, Key: k, TurnID: turn, Args: append(json.RawMessage(nil), args...)}
	if b.tool.Name == "notes.create" || b.tool.Name == "notes.delete" {
		// One mutation of each kind per logical request. Stable identity survives
		// reconnects even if the model reformulates arguments: SQLite rejects the
		// conflict instead of creating a second note. A deliberate new mutation
		// requires a fresh requestId.
		if previous := s.mutationArgs[b.tool.Name]; previous != "" && previous != k {
			return fail("logical_request_conflict")
		}
		j.Key = key(s.namespace, b.tool.Name, json.RawMessage(`null`))
	}
	if b.tool.Name != "notes.list" {
		if cached, ok := s.cache[j.Key]; ok {
			return j, &cached
		}
	}
	if b.tool.Name != "notes.list" && b.tool.Name != "notes.create" {
		p := s.Pending
		if p != nil && time.Now().Before(p.Expires) && p.Name == j.Name && key(s.namespace, p.Name, p.Args) == k && p.Approved {
			j.Confirmed = true
			s.Pending = nil
		} else {
			// Repeated model calls cannot reset the confirmation's original turn.
			if p == nil || p.Name != j.Name || key(s.namespace, p.Name, p.Args) != k || time.Now().After(p.Expires) {
				s.Pending = &Pending{OperationID: id, Name: j.Name, Args: j.Args, TurnID: turn, Expires: time.Now().Add(60 * time.Second)}
			}
			r := mcp.TextResult(map[string]any{"status": "confirmation_required", "operation_id": s.Pending.OperationID, "instruction": "Peça confirmação explícita em um novo turno. Não anuncie sucesso. Após confirmação, chame novamente a mesma ferramenta."}, false)
			return j, &r
		}
	}
	if j.Name == "notes.create" || j.Name == "notes.delete" {
		s.mutationArgs[j.Name] = k
	}
	return j, nil
}

func (s *Session) Execute(ctx context.Context, j Job) mcp.Result {
	result, err := s.backend.Call(ctx, j.Name, j.Args, j.Key, j.Confirmed)
	if err != nil {
		return mcp.Failure("mcp_unavailable_or_timeout")
	}
	// This POC feeds only bounded text results to Nova, not binary resources.
	encoded, _ := json.Marshal(result)
	if len(result.Content) == 0 || len(result.Content) > 16 {
		return mcp.Failure("invalid_result_content")
	}
	if len(encoded) > 65536 {
		return mcp.Failure("result_too_large")
	}
	for _, c := range result.Content {
		if c.Type != "text" {
			return mcp.Failure("unsupported_result_content")
		}
	}
	return result
}
func (s *Session) Complete(j Job, r mcp.Result) {
	if j.Name != "notes.list" && !r.IsError {
		s.cache[j.Key] = r
	}
}

func normalize(text string) string {
	text = strings.ToLower(text)
	text = strings.Map(func(r rune) rune {
		switch r {
		case 'ã', 'á', 'à', 'â':
			return 'a'
		case 'é', 'ê':
			return 'e'
		case 'í':
			return 'i'
		case 'ó', 'ô', 'õ':
			return 'o'
		case 'ú':
			return 'u'
		case 'ç':
			return 'c'
		}
		if unicode.IsPunct(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

// Only a subsequent finalized USER ASR turn may authorize an action. Quoted
// phrases, a bare 'sim', assistant output and tool arguments are insufficient.
func (s *Session) ObserveUser(turn, text string) bool {
	p := s.Pending
	if p == nil || turn == p.TurnID {
		return false
	}
	if time.Now().After(p.Expires) {
		s.Pending = nil
		return false
	}
	phrase := normalize(text)
	expected := "confirmo executar"
	if p.Name == "notes.delete" {
		expected = "confirmo excluir"
	}
	if phrase == expected {
		p.Approved = true
		return true
	}
	if phrase == "nao" || strings.HasPrefix(phrase, "nao ") || phrase == "cancela" || phrase == "cancelar" || phrase == "cancele" {
		s.Pending = nil
	}
	return false
}
func (s *Session) Confirm(operationID string, approved bool) bool {
	p := s.Pending
	if p == nil || p.OperationID != operationID || time.Now().After(p.Expires) {
		return false
	}
	if approved {
		p.Approved = true
	} else {
		s.Pending = nil
	}
	return true
}
func (s *Session) Interrupt() { s.Pending = nil }
