package mcp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// EnvelopeCodec authenticates host decisions transported through LiteLLM's
// arguments. The model sees only Payload's original business schema.
type EnvelopeCodec struct {
	secret   []byte
	audience string
	now      func() time.Time
}

type hostContext struct {
	Version        int    `json:"version"`
	Audience       string `json:"audience"`
	Tool           string `json:"tool"`
	PayloadHash    string `json:"payload_hash"`
	IdempotencyKey string `json:"idempotency_key"`
	Confirmed      bool   `json:"confirmed"`
	ExpiresAt      int64  `json:"expires_at"`
	Signature      string `json:"signature"`
}

type toolEnvelope struct {
	Payload json.RawMessage `json:"payload"`
	Context hostContext     `json:"host_context"`
}

func ServerEnvelope(mode, audience string) (*EnvelopeCodec, error) {
	if mode == "meta" {
		return nil, nil
	}
	if mode != "envelope" {
		return nil, errors.New("unsupported MCP context mode")
	}
	return NewEnvelopeCodec(os.Getenv("STS_MCP_CONTEXT_SECRET"), audience)
}

func NewEnvelopeCodec(secretHex, audience string) (*EnvelopeCodec, error) {
	secret, err := hex.DecodeString(secretHex)
	if err != nil || len(secret) != 32 || !aliasPattern.MatchString(audience) {
		return nil, errors.New("MCP envelope requires a 32-byte hex secret and a valid audience")
	}
	return &EnvelopeCodec{secret: secret, audience: audience, now: time.Now}, nil
}

func canonicalPayload(payload json.RawMessage) (json.RawMessage, string, error) {
	if len(payload) > 16384 {
		return nil, "", errors.New("arguments exceed limit")
	}
	// Decode recursively with UseNumber to avoid large integer precision loss.
	canonical, err := canonicalJSON(payload)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(hash[:]), nil
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func canonicalJSON(data []byte) (json.RawMessage, error) {
	var value map[string]any
	if err := strictDecode(data, &value); err != nil || value == nil {
		return nil, errors.New("arguments must be an object")
	}
	return json.Marshal(value)
}

func (c *EnvelopeCodec) signature(context hostContext) string {
	context.Signature = ""
	data, _ := json.Marshal(context)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte("sts-mcp-envelope-v1\x00"))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *EnvelopeCodec) Wrap(tool string, payload json.RawMessage, key string, confirmed bool) (json.RawMessage, error) {
	canonical, hash, err := canonicalPayload(payload)
	if err != nil {
		return nil, err
	}
	if tool == "" || key == "" || len(key) > 256 {
		return nil, errors.New("invalid host operation")
	}
	context := hostContext{Version: 1, Audience: c.audience, Tool: tool, PayloadHash: hash, IdempotencyKey: key, Confirmed: confirmed, ExpiresAt: c.now().Add(time.Minute).Unix()}
	context.Signature = c.signature(context)
	return json.Marshal(toolEnvelope{Payload: canonical, Context: context})
}

func (c *EnvelopeCodec) Unwrap(tool string, data json.RawMessage) (json.RawMessage, string, bool, error) {
	deny := errors.New("invalid host context")
	if len(data) > 32768 {
		return nil, "", false, deny
	}
	var envelope toolEnvelope
	if err := strictDecode(data, &envelope); err != nil {
		return nil, "", false, deny
	}
	context := envelope.Context
	now := c.now().Unix()
	if context.Version != 1 || context.Audience != c.audience || context.Tool != tool || context.ExpiresAt <= now || context.ExpiresAt > now+60 || context.IdempotencyKey == "" || len(context.IdempotencyKey) > 256 {
		return nil, "", false, deny
	}
	canonical, hash, err := canonicalPayload(envelope.Payload)
	if err != nil || hash != context.PayloadHash {
		return nil, "", false, deny
	}
	signature, err := hex.DecodeString(context.Signature)
	expected, _ := hex.DecodeString(c.signature(context))
	if err != nil || !hmac.Equal(signature, expected) {
		return nil, "", false, deny
	}
	return canonical, context.IdempotencyKey, context.Confirmed, nil
}

func EnvelopeTools(tools []Tool) []Tool {
	wrapped := make([]Tool, len(tools))
	for i, tool := range tools {
		tool.InputSchema, _ = json.Marshal(map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"payload":      tool.InputSchema,
				"host_context": map[string]any{"type": "object"},
			},
			"required": []string{"payload", "host_context"},
		})
		wrapped[i] = tool
	}
	return wrapped
}
