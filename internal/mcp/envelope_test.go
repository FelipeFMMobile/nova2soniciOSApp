package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

const testSecret = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestEnvelopeRoundTripAndTamperResistance(t *testing.T) {
	codec, err := NewEnvelopeCodec(testSecret, "memo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	codec.now = func() time.Time { return now }
	payload := json.RawMessage(`{"content":"x","title":"t"}`)
	envelope, err := codec.Wrap("notes.create", payload, "stable", false)
	if err != nil {
		t.Fatal(err)
	}
	got, key, confirmed, err := codec.Unwrap("notes.create", envelope)
	if err != nil || key != "stable" || confirmed || string(got) != `{"content":"x","title":"t"}` {
		t.Fatal(string(got), key, confirmed, err)
	}

	var changed map[string]any
	_ = json.Unmarshal(envelope, &changed)
	changed["payload"].(map[string]any)["title"] = "changed"
	tampered, _ := json.Marshal(changed)
	if _, _, _, err = codec.Unwrap("notes.create", tampered); err == nil {
		t.Fatal("tampered payload accepted")
	}
	if _, _, _, err = codec.Unwrap("notes.delete", envelope); err == nil {
		t.Fatal("envelope replayed for another tool")
	}
	codec.now = func() time.Time { return now.Add(61 * time.Second) }
	if _, _, _, err = codec.Unwrap("notes.create", envelope); err == nil {
		t.Fatal("expired envelope accepted")
	}
}

func TestEnvelopeRejectsForgedConfirmationAndWrongAudience(t *testing.T) {
	codec, _ := NewEnvelopeCodec(testSecret, "memo")
	envelope, _ := codec.Wrap("notes.delete", json.RawMessage(`{"id":"1"}`), "delete", false)
	var changed map[string]any
	_ = json.Unmarshal(envelope, &changed)
	changed["host_context"].(map[string]any)["confirmed"] = true
	forged, _ := json.Marshal(changed)
	if _, _, _, err := codec.Unwrap("notes.delete", forged); err == nil {
		t.Fatal("forged confirmation accepted")
	}
	other, _ := NewEnvelopeCodec(testSecret, "local")
	if _, _, _, err := other.Unwrap("notes.delete", envelope); err == nil {
		t.Fatal("cross-server replay accepted")
	}
}

func TestEnvelopeToolsKeepBusinessSchemaInsidePayload(t *testing.T) {
	tools := EnvelopeTools(NotesTools())
	var wrapper struct {
		Properties struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"properties"`
	}
	if json.Unmarshal(tools[0].InputSchema, &wrapper) != nil || string(wrapper.Properties.Payload) != string(NotesTools()[0].InputSchema) {
		t.Fatal("business schema not wrapped")
	}
}
