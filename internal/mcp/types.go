// Package mcp implements the bounded stdio tools subset of MCP 2025-11-25.
package mcp

import (
	"encoding/json"
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const ProtocolVersion = "2025-11-25"
const MaxMessage = 256 * 1024

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations map[string]any  `json:"annotations,omitempty"`
}
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

func TextResult(value any, failed bool) Result {
	data, _ := json.Marshal(value)
	return Result{Content: []Content{{Type: "text", Text: string(data)}}, IsError: failed}
}
func Failure(code string) Result {
	return TextResult(map[string]string{"status": "error", "code": code}, true)
}

type noRemote struct{}

func (noRemote) Load(string) (any, error) {
	return nil, errors.New("remote schema references are not supported")
}
func Compile(schema json.RawMessage) (*jsonschema.Schema, error) {
	var doc any
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(noRemote{})
	if err := c.AddResource("urn:sts:tool", doc); err != nil {
		return nil, err
	}
	return c.Compile("urn:sts:tool")
}
func Validate(schema *jsonschema.Schema, args json.RawMessage) error {
	if len(args) > 16384 {
		return errors.New("arguments exceed limit")
	}
	var value any
	if err := json.Unmarshal(args, &value); err != nil {
		return err
	}
	return schema.Validate(value)
}

func NotesTools() []Tool {
	return []Tool{
		{Name: "notes.create", Description: "Cria uma nota persistente com título e conteúdo. Use para registrar informação solicitada pelo usuário.", InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":256},"content":{"type":"string","minLength":1,"maxLength":8192}},"required":["title","content"],"additionalProperties":false}`)},
		{Name: "notes.list", Description: "Busca notas persistentes por trecho do título ou conteúdo. Consulte para responder sobre informações armazenadas; query vazio lista notas.", Annotations: map[string]any{"readOnlyHint": true}, InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","maxLength":256}},"additionalProperties":false}`)},
		{Name: "notes.delete", Description: "Solicita excluir uma nota pelo ID retornado por notes.list. Se receber confirmation_required, peça ao usuário que diga exatamente confirmo excluir. Só anuncie exclusão após status deleted.", Annotations: map[string]any{"destructiveHint": true}, InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","minLength":1,"maxLength":128}},"required":["id"],"additionalProperties":false}`)},
	}
}
