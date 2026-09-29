package agentwire

import (
	"encoding/json"
	"testing"
)

func TestProtocolRejectsUnknownVersionFieldsAndOrphanAudio(t *testing.T) {
	valid, _ := json.Marshal(Message{Protocol: Protocol, Kind: OpenSession, AgentID: "personal-assistant", Start: &AgentSessionStart{SessionID: "session-1", RequestID: "request-1", AgentID: "personal-assistant", Provider: "nova"}})
	message, err := Decode(valid)
	if err != nil || message.ValidateOpen() != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"protocol":"agent-session/v2","kind":"session.open"}`),
		[]byte(`{"protocol":"agent-session/v1","kind":"session.open","unknown":true}`),
	} {
		if _, err := Decode(raw); err == nil {
			t.Fatalf("invalid message accepted: %s", raw)
		}
	}
	orphan := Message{Protocol: Protocol, Kind: SessionCommand, Command: &AgentCommand{Type: InputAudio, SessionID: "other", TurnID: "turn-1", Sequence: 1, Audio: []byte{0, 0}}}
	if orphan.ValidateCommand("session-1") == nil {
		t.Fatal("orphan audio accepted")
	}
	gap := Message{Protocol: Protocol, Kind: SessionCommand, Command: &AgentCommand{Type: InputAudio, SessionID: "session-1", TurnID: "turn-1", Sequence: 0, Audio: []byte{0, 0}}}
	if gap.ValidateCommand("session-1") == nil {
		t.Fatal("zero audio sequence accepted")
	}
}

func TestProtocolRejectsOversizedAudio(t *testing.T) {
	message := Message{Protocol: Protocol, Kind: SessionCommand, Command: &AgentCommand{Type: InputAudio, SessionID: "session-1", TurnID: "turn-1", Sequence: 1, Audio: make([]byte, MaxAudioBytes+2)}}
	if message.ValidateCommand("session-1") == nil {
		t.Fatal("oversized audio accepted")
	}
}
