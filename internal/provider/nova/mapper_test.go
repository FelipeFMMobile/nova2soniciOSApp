package nova

import (
	"testing"

	"stsmodel.local/poc/internal/protocol"
)

func mapped(t *testing.T, m *Mapper, e Event) []protocol.Event {
	t.Helper()
	events, err := m.Map(e)
	if err != nil {
		t.Fatal(err)
	}
	return events
}
func user(t *testing.T, m *Mapper, id, text string) {
	t.Helper()
	mapped(t, m, Event{Kind: "contentStart", ContentID: id, Type: "TEXT", Role: "USER", Stage: "FINAL"})
	mapped(t, m, Event{Kind: "textOutput", ContentID: id, Text: text})
	mapped(t, m, Event{Kind: "contentEnd", ContentID: id, StopReason: "END_TURN"})
}
func audioStart(t *testing.T, m *Mapper, id string) {
	t.Helper()
	mapped(t, m, Event{Kind: "contentStart", ContentID: id, Type: "AUDIO", Role: "ASSISTANT", Rate: 24000})
}

func TestMapperInterruptionAndStaleContent(t *testing.T) {
	m := NewMapper(nil)
	m.StartInput("a")
	user(t, m, "u-a", "Olá")
	audioStart(t, m, "a-a")
	if e := mapped(t, m, Event{Kind: "audioOutput", ContentID: "a-a", Audio: "AAAAAA=="}); len(e) != 1 || e[0].TurnID != "a" || e[0].Sequence != 1 {
		t.Fatal(e)
	}
	if e := m.StartInput("b"); e[0].Type != protocol.TurnInterrupted {
		t.Fatal(e)
	}
	if e := mapped(t, m, Event{Kind: "audioOutput", ContentID: "a-a", Audio: "AAAAAA=="}); len(e) != 0 {
		t.Fatal("stale audio passed", e)
	}
	// Newly queued old content cannot be attached to the new turn before its ASR.
	audioStart(t, m, "late-a")
	if e := mapped(t, m, Event{Kind: "audioOutput", ContentID: "late-a", Audio: "AAAAAA=="}); len(e) != 0 {
		t.Fatal("old generation revived")
	}
	user(t, m, "u-b", "Novo assunto")
	audioStart(t, m, "a-b")
	if e := mapped(t, m, Event{Kind: "audioOutput", ContentID: "a-b", Audio: "AAAAAA=="}); len(e) != 1 || e[0].TurnID != "b" || e[0].Sequence != 1 {
		t.Fatal(e)
	}
	e := mapped(t, m, Event{Kind: "contentEnd", ContentID: "a-b", StopReason: "END_TURN"})
	if len(e) != 2 || e[0].Type != protocol.TurnCompleted || e[0].Metrics == nil {
		t.Fatal(e)
	}
}

func TestHistoryContainsOnlyFinalSpeech(t *testing.T) {
	m := NewMapper(nil)
	m.StartInput("a")
	user(t, m, "u", "Pergunta")
	user(t, m, "u-fragment", "complemento")
	for _, stage := range []string{"SPECULATIVE", "FINAL"} {
		id := stage
		mapped(t, m, Event{Kind: "contentStart", ContentID: id, Type: "TEXT", Role: "ASSISTANT", Stage: stage})
		mapped(t, m, Event{Kind: "textOutput", ContentID: id, Text: stage})
		mapped(t, m, Event{Kind: "contentEnd", ContentID: id, StopReason: "END_TURN"})
	}
	if len(m.History) != 2 || m.History[1].Content != "FINAL" || m.History[0].Content != "Pergunta complemento" {
		t.Fatal(m.History)
	}
}

func TestInterruptedSignalAndInvalidPCM(t *testing.T) {
	e, err := decode("textOutput", []byte(`{"contentId":"c","content":"{\"interrupted\":true}"}`))
	if err != nil || e.Kind != "interrupted" {
		t.Fatal(e, err)
	}
	m := NewMapper(nil)
	m.StartInput("a")
	user(t, m, "u", "oi")
	audioStart(t, m, "a")
	if _, err := m.Map(Event{Kind: "audioOutput", ContentID: "a", Audio: "AAAA"}); err == nil {
		t.Fatal("accepted odd PCM")
	}
}
