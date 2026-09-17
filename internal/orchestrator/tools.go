// Package orchestrator owns tool permission decisions, not the model or bridge.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
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
	tool     mcp.Tool
	schema   *jsonschema.Schema
	policy   mcp.Policy
	original string
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
	intentTurn string
	intentText string
	backend    Backend
	tools      map[string]binding
	Specs      []map[string]any
	namespace  string
	Pending    *Pending
	seen       map[string]string
	cache      map[string]mcp.Result
	calls      int
	attempted  map[string]bool
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
	s := &Session{backend: backend, tools: map[string]binding{}, namespace: namespace, seen: map[string]string{}, cache: map[string]mcp.Result{}, attempted: map[string]bool{}}
	for _, tool := range discovered {
		if !permit[tool.Name] {
			continue
		}
		name := strings.ReplaceAll(strings.ReplaceAll(tool.Name, ".", "_"), "-", "_")
		if !modelToolName.MatchString(name) || len(name) > 64 || len(tool.Description) > 4096 {
			return nil, errors.New("unsupported tool metadata")
		}
		if _, duplicate := s.tools[name]; duplicate {
			return nil, errors.New("tool name mapping collision")
		}
		schema, err := mcp.Compile(tool.InputSchema)
		if err != nil {
			return nil, errors.New("invalid MCP tool schema")
		}
		original := tool.OriginalName
		if original == "" {
			original = tool.Name
		}
		policy := tool.HostPolicy
		if policy == "" {
			switch original {
			case "notes.list":
				policy = mcp.ReadOnly
			case "notes.create", "agenda.create_event":
				policy = mcp.ExplicitIntent
			default:
				policy = mcp.ConfirmLater
			}
		}
		s.tools[name] = binding{tool: tool, schema: schema, policy: policy, original: original}
		// Explicit approval policy lives here, not in untrusted MCP annotations.
		description := tool.Description
		if policy == mcp.ConfirmLater {
			description += " Esta ação exige confirmação do usuário em um turno posterior. Se receber confirmation_required, peça confirmo excluir (notas), confirmo cancelar agendamento (agenda) ou confirmo executar, depois chame novamente com os mesmos argumentos."
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
		resolved := name
		if b, ok := s.tools[name]; ok {
			resolved = b.tool.Name
		}
		return Job{ID: id, Name: resolved, TurnID: turn, Args: append(json.RawMessage(nil), args...)}, &r
	}
	if !protocol.ValidID(id) {
		return fail("invalid_tool_id")
	}
	b, ok := s.tools[name]
	if !ok {
		return fail("tool_not_allowed")
	}
	if b.original == "agenda.create_event" || b.original == "agenda.list_slots" || b.original == "agenda.list_events" {
		if result := agendaFormatError(args); result != nil {
			return Job{ID: id, Name: b.tool.Name, TurnID: turn, Args: append(json.RawMessage(nil), args...)}, result
		}
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
	// Each distinct canonical argument set is an independent operation. Keep
	// its key stable across model tool IDs, turns and reconnects, so identical
	// retries cannot duplicate effects. Changed arguments are a new operation,
	// not a safe retry of an unknown outcome.
	if b.policy != mcp.ReadOnly {
		if cached, ok := s.cache[j.Key]; ok {
			return j, &cached
		}
	}
	if b.policy != mcp.ReadOnly && s.attempted[j.Key] {
		return fail("mutation_outcome_pending_or_unknown")
	}
	if b.policy == mcp.ConfirmLater {
		p := s.Pending
		if p != nil && time.Now().Before(p.Expires) && p.Name == j.Name && key(s.namespace, p.Name, p.Args) == k && p.Approved {
			j.Confirmed = true
			s.Pending = nil
		} else {
			// Repeated model calls cannot reset the confirmation's original turn.
			if p == nil || p.Name != j.Name || key(s.namespace, p.Name, p.Args) != k || time.Now().After(p.Expires) {
				s.Pending = &Pending{OperationID: id, Name: j.Name, Args: j.Args, TurnID: turn, Expires: time.Now().Add(60 * time.Second)}
			}
			r := mcp.TextResult(map[string]any{"status": "confirmation_required", "operation_id": s.Pending.OperationID, "target": json.RawMessage(s.Pending.Args), "instruction": "Peça confirmação explícita em um novo turno: " + s.ConfirmationPhrase() + ". Não anuncie sucesso. Após confirmação, chame novamente a mesma ferramenta com o mesmo alvo."}, false)
			return j, &r
		}
	}
	// Built-in agenda creation is direct for this POC: trust the model's tool
	// selection, while preserving schema, dates, storage rules and idempotency.
	if b.policy == mcp.ExplicitIntent && b.original != "notes.create" && b.original != "agenda.create_event" && (s.intentTurn != turn || !explicitAgendaIntent(s.intentText)) {
		return fail("clarification_required")
	}
	if b.policy != mcp.ReadOnly {
		s.attempted[j.Key] = true
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
	var policy mcp.Policy
	for _, b := range s.tools {
		if b.tool.Name == j.Name {
			policy = b.policy
			break
		}
	}
	if policy != mcp.ReadOnly {
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
	s.intentTurn = turn
	s.intentText = normalize(text)
	if strings.ContainsAny(text, "\"'“”‘’?") {
		s.intentText = ""
	}
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
	original := p.Name
	for _, b := range s.tools {
		if b.tool.Name == p.Name {
			original = b.original
		}
	}
	if original == "notes.delete" {
		expected = "confirmo excluir"
	}
	if original == "agenda.cancel_event" {
		expected = "confirmo cancelar agendamento"
	}
	if phrase == expected && !strings.ContainsAny(text, "\"'“”‘’?") {
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
	for _, b := range s.tools {
		if b.tool.Name == p.Name && b.original == "agenda.cancel_event" {
			return false
		}
	}
	if approved {
		p.Approved = true
	} else {
		s.Pending = nil
	}
	return true
}
func (s *Session) Interrupt() { s.Pending = nil; s.intentText = "" }

// Conservative PT-BR terminal POC grammar. Intent comes only from finalized
// USER ASR, never annotations, arguments, model output or quoted text.
var agendaIntent = regexp.MustCompile(`^(por favor )?((consulte|busque|leia) .+ e )?(agende|agenda|marque|crie um agendamento|crie um evento|quero agendar|quero marcar) .+`)

func explicitAgendaIntent(text string) bool {
	if !agendaIntent.MatchString(text) {
		return false
	}
	for _, word := range []string{"nao", "talvez", "se", "disse", "algum"} {
		for _, w := range strings.Fields(text) {
			if w == word {
				return false
			}
		}
	}
	return true
}

var modelToolName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

func (s *Session) ConfirmationPhrase() string {
	if s.Pending == nil {
		return "confirmo executar"
	}
	for _, b := range s.tools {
		if b.tool.Name == s.Pending.Name {
			switch b.original {
			case "notes.delete":
				return "confirmo excluir"
			case "agenda.cancel_event":
				return "confirmo cancelar agendamento"
			}
		}
	}
	return "confirmo executar"
}
