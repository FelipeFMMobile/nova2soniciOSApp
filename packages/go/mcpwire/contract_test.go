package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPublishedHostContextVectors(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "../../../apps/voice-gateway/contracts/host-context/v1/vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Now                         int64
		Secret, Audience, Tool, Key string
		Confirmed                   bool
		Payload, Envelope           json.RawMessage
	}
	if err = json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		c, err := NewEnvelopeCodec(v.Secret, v.Audience)
		if err != nil {
			t.Fatal(err)
		}
		c.now = func() time.Time { return time.Unix(v.Now, 0) }
		got, err := c.Wrap(v.Tool, v.Payload, v.Key, v.Confirmed)
		if err != nil {
			t.Fatal(err)
		}
		var actual, expected toolEnvelope
		if json.Unmarshal(got, &actual) != nil || json.Unmarshal(v.Envelope, &expected) != nil {
			t.Fatal("bad fixture")
		}
		if actual.Context != expected.Context {
			t.Fatal("wire signature changed", actual.Context, expected.Context)
		}
		payload, key, confirmed, err := c.Unwrap(v.Tool, v.Envelope)
		if err != nil || key != v.Key || confirmed != v.Confirmed {
			t.Fatal(err)
		}
		canonical, _, _ := canonicalPayload(v.Payload)
		if !bytes.Equal(payload, canonical) {
			t.Fatal("payload changed")
		}
		for _, bad := range []json.RawMessage{bytes.Replace(v.Envelope, []byte(`"stable"`), []byte(`"tampered"`), 1), bytes.Replace(v.Envelope, []byte(`"memo"`), []byte(`"other"`), 1)} {
			if _, _, _, err = c.Unwrap(v.Tool, bad); err == nil {
				t.Fatal("tamper accepted")
			}
		}
	}
}
