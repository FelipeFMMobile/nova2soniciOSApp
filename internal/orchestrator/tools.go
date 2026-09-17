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
	intentTurn   string
	intentText   string
	backend      Backend
	tools        map[string]binding
	Specs        []map[string]any
	namespace    string
	Pending      *Pending
	seen         map[string]string
	cache        map[string]mcp.Result
	calls        int
	attempted    map[string]bool
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
	s := &Session{backend: backend, tools: map[string]binding{}, namespace: namespace, seen: map[string]string{}, cache: map[string]mcp.Result{}, mutationArgs: map[string]string{}, attempted: map[string]bool{}}
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
			case "notes.create":
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
	if mcp.Validate(b.schema, args) != nil {
		if b.original == "agenda.create_event" {
			r := mcp.TextResult(map[string]any{"status": "error", "code": "invalid_arguments", "instruction": "Envie title, start e end. start/end devem ser strings RFC3339 com ano, segundos e offset, por exemplo 2030-05-20T09:00:00-03:00 e 2030-05-20T10:00:00-03:00. Não envie texto por extenso. Esclareça campos ausentes com o usuário e chame novamente a ferramenta; não anuncie sucesso."}, true)
			return Job{ID: id, Name: b.tool.Name, TurnID: turn}, &r
		}
		return fail("invalid_arguments")
	}
	if b.original == "agenda.create_event" {
		var interval struct {
			Start string `json:"start"`
			End   string `json:"end"`
		}
		_ = json.Unmarshal(args, &interval)
		start, errStart := time.Parse(time.RFC3339, interval.Start)
		end, errEnd := time.Parse(time.RFC3339, interval.End)
		if errStart != nil || errEnd != nil || !start.Before(end) || start.Nanosecond() != 0 || end.Nanosecond() != 0 || end.Sub(start) > 8*time.Hour {
			r := mcp.TextResult(map[string]any{"status": "error", "code": "invalid_datetime", "instruction": "Corrija start/end para RFC3339 com offset explícito (AAAA-MM-DDTHH:MM:SS±HH:MM), sem frações. Exemplo para uma hora: 2030-05-20T09:00:00-03:00 até 2030-05-20T10:00:00-03:00. O fim deve ser posterior ao início e duração máxima é oito horas. Não copie a data do exemplo; esclareça ano/data/duração ausente com o usuário. Depois chame novamente com os argumentos corrigidos; não anuncie sucesso."}, true)
			return Job{ID: id, Name: b.tool.Name, TurnID: turn}, &r
		}
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
	if b.policy != mcp.ReadOnly {
		// One mutation of each kind per logical request. Stable identity survives
		// reconnects even if the model reformulates arguments: SQLite rejects the
		// conflict instead of creating a second note. A deliberate new mutation
		// requires a fresh requestId.
		if previous := s.mutationArgs[b.tool.Name]; previous != "" && previous != k {
			return fail("logical_request_conflict")
		}
		j.Key = key(s.namespace, b.tool.Name, json.RawMessage(`null`))
	}
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
	if b.original == "agenda.create_event" && !(s.intentTurn == turn && explicitAgendaIntent(s.intentText)) {
		p := s.Pending
		matching := p != nil && p.Name == j.Name && time.Now().Before(p.Expires) && key(s.namespace, p.Name, p.Args) == k
		if matching && p.Approved && turn != p.TurnID {
			j.Confirmed = true
			s.Pending = nil
		} else if matching || (s.intentTurn == turn && spokenAgendaIntent(s.intentText)) {
			if !matching {
				s.Pending = &Pending{OperationID: id, Name: j.Name, Args: j.Args, TurnID: turn, Expires: time.Now().Add(60 * time.Second)}
			}
			r := mcp.TextResult(map[string]any{"status": "confirmation_required", "operation_id": s.Pending.OperationID, "target": json.RawMessage(s.Pending.Args), "timezone": "America/Sao_Paulo", "instruction": "Ainda não há evento criado. Leia o título, data completa com ano, início, fim e fuso da proposta target. Peça ao usuário que diga exatamente Confirmo agendar em outro turno. Não peça apenas sim. Depois da confirmação, chame novamente agenda.create_event com os MESMOS argumentos. Se o usuário corrigir dados, esclareça e peça um novo pedido explícito de agendamento; a confirmação antiga não vale para outro alvo."}, false)
			return j, &r
		} else {
			r := mcp.TextResult(map[string]any{"status": "error", "code": "clarification_required", "instruction": "Peça um pedido explícito do usuário começando com agende ou quero agendar, incluindo título, data e duração. Datas faladas são permitidas como proposta; envie os argumentos em RFC3339 e aguarde confirmation_required. Não basta pedir sim, nem usar a fala do assistente como autorização."}, true)
			return j, &r
		}
	} else if b.policy == mcp.ExplicitIntent && b.original != "notes.create" && b.original != "agenda.create_event" && (s.intentTurn != turn || !explicitAgendaIntent(s.intentText)) {
		return fail("clarification_required")
	}
	if b.policy != mcp.ReadOnly {
		s.mutationArgs[j.Name] = k
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
	if original == "agenda.create_event" {
		expected = "confirmo agendar"
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
		if b.tool.Name == p.Name && (b.original == "agenda.cancel_event" || b.original == "agenda.create_event") {
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
var agendaDate = regexp.MustCompile(`\b\d{4} \d{2} \d{2}\b|\b\d{2} \d{2} \d{4}\b`)
var agendaClock = regexp.MustCompile(`\b\d{1,2} \d{2}\b|\b\d{1,2}h(\d{2})?\b`)

func explicitAgendaIntent(text string) bool {
	if !agendaIntent.MatchString(text) || !agendaDate.MatchString(text) || !agendaClock.MatchString(text) {
		return false
	}
	for _, word := range []string{"nao", "talvez", "se", "disse", "amanha", "depois", "algum"} {
		for _, w := range strings.Fields(text) {
			if w == word {
				return false
			}
		}
	}
	return true
}

// Spoken dates are proposals, never immediate authorization. Refuse negation,
// uncertainty and quoted requests; execution needs a later target-bound ASR confirmation.
func spokenAgendaIntent(text string) bool {
	if !agendaIntent.MatchString(text) {
		return false
	}
	for _, word := range strings.Fields(text) {
		switch word {
		case "nao", "talvez", "se", "disse", "algum":
			return false
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
			if b.original == "agenda.create_event" {
				return "confirmo agendar"
			}
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
