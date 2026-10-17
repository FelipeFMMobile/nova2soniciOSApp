package mcp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

// IdentityContext is asserted by an authenticated host, never supplied by a model.
// SessionRef is a non-bearer reference: possession does not authenticate a caller.
type IdentityContext struct {
	Subject    string   `json:"subject"`
	ClientCode string   `json:"client_code"`
	SessionRef string   `json:"session_ref"`
	DeviceID   string   `json:"device_id"`
	Scopes     []string `json:"scopes"`
}
type HostContextV2 struct {
	Version        int             `json:"version"`
	Audience       string          `json:"audience"`
	Tool           string          `json:"tool"`
	PayloadHash    string          `json:"payload_hash"`
	IdempotencyKey string          `json:"idempotency_key"`
	Confirmed      bool            `json:"confirmed"`
	ExpiresAt      int64           `json:"expires_at"`
	Identity       IdentityContext `json:"identity"`
	Signature      string          `json:"signature"`
}
type EnvelopeV2 struct {
	Payload json.RawMessage `json:"payload"`
	Context HostContextV2   `json:"host_context"`
}

func (c *EnvelopeCodec) signV2(context HostContextV2) string {
	context.Signature = ""
	b, _ := json.Marshal(context)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte("sts-mcp-envelope-v2\x00"))
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil))
}
func (c *EnvelopeCodec) WrapV2(tool string, payload json.RawMessage, key string, confirmed bool, identity IdentityContext) (json.RawMessage, error) {
	canonical, hash, err := canonicalV2(payload)
	if err != nil {
		return nil, err
	}
	if tool == "" || key == "" || len(key) > 256 || identity.Subject == "" || identity.DeviceID == "" {
		return nil, errors.New("invalid host operation")
	}
	if identity.Scopes == nil {
		identity.Scopes = []string{}
	}
	context := HostContextV2{2, c.audience, tool, hash, key, confirmed, c.now().Add(time.Minute).Unix(), identity, ""}
	context.Signature = c.signV2(context)
	return json.Marshal(EnvelopeV2{canonical, context})
}
func (c *EnvelopeCodec) UnwrapV2(tool string, data json.RawMessage) (EnvelopeV2, error) {
	deny := errors.New("invalid host context")
	var e EnvelopeV2
	if len(data) > 32768 || strictDecode(data, &e) != nil {
		return e, deny
	}
	h := e.Context
	now := c.now().Unix()
	if h.Version != 2 || h.Audience != c.audience || h.Tool != tool || h.ExpiresAt <= now || h.ExpiresAt > now+60 || h.IdempotencyKey == "" || len(h.IdempotencyKey) > 256 || h.Identity.Subject == "" || h.Identity.DeviceID == "" || h.Identity.Scopes == nil {
		return e, deny
	}
	_, hash, err := canonicalV2(e.Payload)
	if err != nil || hash != h.PayloadHash {
		return e, deny
	}
	signature, err := hex.DecodeString(h.Signature)
	expected, _ := hex.DecodeString(c.signV2(h))
	if err != nil || !hmac.Equal(signature, expected) {
		return e, deny
	}
	return e, nil
}

// v2 limits canonical numbers to integers and object keys to ASCII. This is
// sufficient for the account contracts and is unambiguous across Go/Jackson.
func canonicalV2(data json.RawMessage) (json.RawMessage, string, error) {
	var value any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if d.Decode(&value) != nil {
		return nil, "", errors.New("invalid payload")
	}
	var valid func(any) bool
	valid = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				for _, r := range k {
					if r > 127 {
						return false
					}
				}
				if !valid(c) {
					return false
				}
			}
		case []any:
			for _, c := range x {
				if !valid(c) {
					return false
				}
			}
		case json.Number:
			return regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`).MatchString(string(x))
		}
		return true
	}
	if !valid(value) {
		return nil, "", errors.New("v2 payload requires ASCII keys and integer numbers")
	}
	return canonicalPayload(data)
}
