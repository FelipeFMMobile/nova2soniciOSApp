package session

import (
	"encoding/base64"
	"testing"

	"stsmodel.local/poc/internal/protocol"
)

func frame(s *Session, turn string, seq uint64) protocol.Event {
	return protocol.Event{SessionID: s.ID, TurnID: turn, Sequence: seq, Audio: base64.StdEncoding.EncodeToString([]byte{0, 0}), SampleRate: 16000}
}

func TestLifecycleAndStaleTurns(t *testing.T) {
	s := New()
	if _, err := s.Append(frame(s, "a", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(frame(s, "a", 1)); err == nil {
		t.Fatal("accepted duplicate sequence")
	}
	if _, err := s.Commit("a"); err != nil {
		t.Fatal(err)
	}
	if old, err := s.Append(frame(s, "b", 2)); err == nil || old != "" || s.TurnID != "a" {
		t.Fatal("malformed barge-in changed state")
	}
	if old, err := s.Append(frame(s, "b", 1)); err != nil || old != "a" {
		t.Fatal("barge-in failed", old, err)
	}
	if s.Complete("a") {
		t.Fatal("stale completion accepted")
	}
	if err := s.Cancel("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(frame(s, "a", 1)); err == nil {
		t.Fatal("cancelled turn reused")
	}
	s.Close()
	if _, err := s.Append(frame(s, "c", 1)); err == nil {
		t.Fatal("closed session accepted input")
	}
}

func TestAudioLimit(t *testing.T) {
	s := New()
	e := frame(s, "a", 1)
	e.Audio = base64.StdEncoding.EncodeToString(make([]byte, protocol.MaxAudioChunkBytes))
	for i := 1; i <= 30; i++ {
		e.Sequence = uint64(i)
		if _, err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	e.Sequence = 31
	if _, err := s.Append(e); err == nil {
		t.Fatal("unbounded audio")
	}
}
