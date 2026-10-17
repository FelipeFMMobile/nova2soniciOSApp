package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEnvelopeV2BindingsAndCanonicalization(t *testing.T) {
	c, _ := NewEnvelopeCodec(strings.Repeat("cc", 32), "profile")
	c.now = func() time.Time { return time.Unix(2000000000, 0) }
	identity := IdentityContext{Subject: "alice", ClientCode: strings.Repeat("a", 64), SessionRef: "session", DeviceID: strings.Repeat("b", 64), Scopes: []string{"profile:read", "profile:write"}}
	first, e := c.WrapV2("profile.update_me", json.RawMessage(`{"changes":{"name":"Á <&>"},"expected_version":1}`), "operation", true, identity)
	if e != nil {
		t.Fatal(e)
	}
	golden, err := os.ReadFile("../../../apps/mcp-profile/contracts/v2/envelope-vector.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, bytes.TrimSpace(golden)) {
		t.Fatalf("Go/Java vector mismatch: %s", first)
	}
	second, _ := c.WrapV2("profile.update_me", json.RawMessage(`{"expected_version":1,"changes":{"name":"Á <&>"}}`), "operation", true, identity)
	if string(first) != string(second) {
		t.Fatal("canonicalization differs")
	}
	if _, e = c.UnwrapV2("profile.update_me", first); e != nil {
		t.Fatal(e)
	}
	for _, replacement := range [][2]string{{"alice", "bruno"}, {"session", "other"}, {"operation", "replay"}, {`"confirmed":true`, `"confirmed":false`}, {`"expected_version":1`, `"expected_version":2`}, {`"profile:write"`, `"admin"`}} {
		changed := strings.Replace(string(first), replacement[0], replacement[1], 1)
		if _, e = c.UnwrapV2("profile.update_me", []byte(changed)); e == nil {
			t.Fatalf("accepted tampering %s", replacement[0])
		}
	}
	if _, e = c.UnwrapV2("profile.get_me", first); e == nil {
		t.Fatal("tool substitution accepted")
	}
	c.now = func() time.Time { return time.Unix(2000000061, 0) }
	if _, e = c.UnwrapV2("profile.update_me", first); e == nil {
		t.Fatal("expired accepted")
	}
}
func TestEnvelopeV2RejectsAmbiguousNumbers(t *testing.T) {
	for _, p := range []string{`{"age":1.0}`, `{"age":1e2}`, `{"á":1}`} {
		if _, _, e := canonicalV2([]byte(p)); e == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}

func TestEnvelopeV2ExplicitRemovalMatchesJavaVector(t *testing.T) {
	c, _ := NewEnvelopeCodec(strings.Repeat("cc", 32), "profile")
	c.now = func() time.Time { return time.Unix(2000000000, 0) }
	identity := IdentityContext{Subject: "alice", ClientCode: strings.Repeat("a", 64), SessionRef: "session", DeviceID: strings.Repeat("b", 64), Scopes: []string{"profile:read", "profile:write"}}
	actual, err := c.WrapV2("profile.update_me", json.RawMessage(`{"expected_version":1,"changes":{"name":"Á <&>","gender":null}}`), "operation", true, identity)
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("../../../apps/mcp-profile/contracts/v2/envelope-null-vector.json")
	if err != nil {
		t.Fatal(err)
	}
	var want EnvelopeV2
	if err = json.Unmarshal(golden, &want); err != nil {
		t.Fatal(err)
	}
	var got EnvelopeV2
	if err = json.Unmarshal(actual, &got); err != nil {
		t.Fatal(err)
	}
	normalized, _ := json.Marshal(want.Context)
	actualContext, _ := json.Marshal(got.Context)
	if !bytes.Equal(actualContext, normalized) {
		t.Fatal("null removal Go/Java vector differs")
	}
	if _, err = c.UnwrapV2("profile.update_me", actual); err != nil {
		t.Fatal(err)
	}
}
