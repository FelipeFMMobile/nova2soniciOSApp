package orchestrator

import (
	"context"
	"encoding/json"
	"stsmodel.local/poc/internal/mcp"
	"testing"
)

type multiFake struct {
	tools []mcp.Tool
	calls int
}

func (f *multiFake) Tools(context.Context) ([]mcp.Tool, error) { return f.tools, nil }
func (f *multiFake) Call(context.Context, string, json.RawMessage, string, bool) (mcp.Result, error) {
	f.calls++
	return mcp.TextResult(map[string]string{"status": "created"}, false), nil
}
func multiSetup(t *testing.T) (*Session, *multiFake) {
	t.Helper()
	f := &multiFake{}
	allowed := []string{}
	for _, group := range []struct {
		alias string
		tools []mcp.Tool
	}{{"memo", mcp.NotesTools()}, {"local", mcp.AgendaTools()}} {
		for _, tool := range group.tools {
			tool.OriginalName = tool.Name
			tool.Server = group.alias
			tool.Name = group.alias + "." + tool.Name
			switch tool.OriginalName {
			case "notes.list", "agenda.list_events", "agenda.list_slots":
				tool.HostPolicy = mcp.ReadOnly
			case "notes.create", "agenda.create_event":
				tool.HostPolicy = mcp.ExplicitIntent
			default:
				tool.HostPolicy = mcp.ConfirmLater
			}
			f.tools = append(f.tools, tool)
			allowed = append(allowed, tool.Name)
		}
	}
	s, err := New(context.Background(), f, allowed, "request")
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}
func TestAgendaHostIntentAndNamespace(t *testing.T) {
	s, f := multiSetup(t)
	args := json.RawMessage(`{"title":"x","start":"2030-05-20T09:00:00-03:00","end":"2030-05-20T10:00:00-03:00"}`)
	s.ObserveUser("u", "Agende teste em 2030-05-20 às 09:00 por uma hora")
	agenda, r := s.Plan("agenda", "local_agenda_create_event", "u", args)
	if r != nil {
		t.Fatal(r)
	}
	_, r = s.Plan("duplicate", "local_agenda_create_event", "u", args)
	if r == nil || !r.IsError {
		t.Fatal("in-flight mutation retried")
	}
	notes, r := s.Plan("notes", "memo_notes_create", "u", json.RawMessage(`{"title":"x","content":"y"}`))
	if r != nil || notes.Key == agenda.Key {
		t.Fatal("namespace collision", r)
	}
	s.Complete(agenda, s.Execute(context.Background(), agenda))
	_, r = s.Plan("retry", "local_agenda_create_event", "u", args)
	if r == nil || r.IsError || f.calls != 1 {
		t.Fatal("replay", r)
	}
}
func TestAgendaCancelLaterRefusalAndTarget(t *testing.T) {
	s, _ := multiSetup(t)
	args := json.RawMessage(`{"id":"event-1"}`)
	s.Plan("c1", "local_agenda_cancel_event", "u1", args)
	if s.Confirm("c1", true) || s.ObserveUser("u1", "confirmo cancelar agendamento") || s.ObserveUser("u2", "sim") {
		t.Fatal("bypass")
	}
	s.ObserveUser("u2", "não")
	if s.Pending != nil {
		t.Fatal("refusal")
	}
	s.Plan("c2", "local_agenda_cancel_event", "u2", args)
	if !s.ObserveUser("u3", "Confirmo cancelar agendamento.") {
		t.Fatal("confirmation")
	}
	_, r := s.Plan("changed", "local_agenda_cancel_event", "u3", json.RawMessage(`{"id":"event-2"}`))
	if r == nil || s.Pending.Approved {
		t.Fatal("target changed")
	}
	s.Plan("c3", "local_agenda_cancel_event", "u3", args)
	s.ObserveUser("u4", "confirmo cancelar agendamento")
	j, r := s.Plan("c4", "local_agenda_cancel_event", "u4", args)
	if r != nil || !j.Confirmed {
		t.Fatal(j, r)
	}
}
func TestCollisionNamesAndWireUnion(t *testing.T) {
	s, _ := multiSetup(t)
	if len(s.Specs) != 7 {
		t.Fatal("union")
	}
	for _, v := range s.Specs {
		spec := v["toolSpec"].(map[string]any)
		wire, ok := spec["inputSchema"].(map[string]any)["json"].(string)
		if !ok || !json.Valid([]byte(wire)) {
			t.Fatal("wire")
		}
	}
	f := &multiFake{tools: []mcp.Tool{{Name: "a.b", InputSchema: json.RawMessage(`{}`)}, {Name: "a_b", InputSchema: json.RawMessage(`{}`)}}}
	if _, err := New(context.Background(), f, []string{"a.b", "a_b"}, "n"); err == nil {
		t.Fatal("collision accepted")
	}
}

func TestMutationFailureDoesNotAutomaticallyExecuteAgain(t *testing.T) {
	s, b := setup(t)
	b.fail = true
	args := json.RawMessage(`{"title":"x","content":"y"}`)
	j, r := s.Plan("first", "notes_create", "u", args)
	if r != nil {
		t.Fatal(r)
	}
	s.Complete(j, s.Execute(context.Background(), j))
	_, r = s.Plan("retry", "notes_create", "u", args)
	if r == nil || !r.IsError || b.calls != 1 {
		t.Fatal("failed mutation retried", r, b.calls)
	}
}
