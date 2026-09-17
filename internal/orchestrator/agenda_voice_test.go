package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"stsmodel.local/poc/internal/mcp"
)

func TestSpokenAgendaRequiresLaterTargetBoundConfirmation(t *testing.T) {
	s, backend := multiSetup(t)
	args := json.RawMessage(`{"title":"pocket","start":"2026-09-18T10:00:00-03:00","end":"2026-09-18T11:00:00-03:00"}`)
	s.ObserveUser("u1", "Agende pocket amanhã às dez da manhã por uma hora")
	_, result := s.Plan("proposal", "local_agenda_create_event", "u1", args)
	if result == nil || result.IsError || mcp.Status(*result) != "confirmation_required" || backend.calls != 0 {
		t.Fatal("proposal must not execute", result)
	}
	if s.ConfirmationPhrase() != "confirmo agendar" || s.Confirm("proposal", true) || s.ObserveUser("u1", "Confirmo agendar") || s.ObserveUser("u2", "sim") {
		t.Fatal("confirmation bypass")
	}
	if !s.ObserveUser("u3", "Confirmo agendar.") {
		t.Fatal("expected finalized voice approval")
	}
	changed := json.RawMessage(`{"title":"pocket","start":"2026-09-18T12:00:00-03:00","end":"2026-09-18T13:00:00-03:00"}`)
	_, result = s.Plan("changed", "local_agenda_create_event", "u3", changed)
	if result == nil || backend.calls != 0 {
		t.Fatal("changed target must not execute")
	}
	job, result := s.Plan("execute", "local_agenda_create_event", "u3", args)
	if result != nil || !job.Confirmed {
		t.Fatal("confirmed target must execute", result)
	}
	s.Complete(job, s.Execute(context.Background(), job))
	_, result = s.Plan("retry", "local_agenda_create_event", "u3", args)
	if result == nil || result.IsError || backend.calls != 1 {
		t.Fatal("retry must replay once", result)
	}
}

func TestMalformedAgendaDateReturnsActionableErrorWithoutExecuting(t *testing.T) {
	s, backend := multiSetup(t)
	s.ObserveUser("u1", "Agende pocket amanhã às dez")
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"title":"pocket","start":"18 de setembro de 2026","end":"18 de setembro de 2026"}`),
		json.RawMessage(`{"title":"pocket","start":"2026-09-18T10:00:00-03:00","end":"2026-09-18T09:00:00-03:00"}`),
	} {
		_, result := s.Plan("bad", "local_agenda_create_event", "u1", args)
		if result == nil || !result.IsError || !strings.Contains(result.Content[0].Text, "RFC3339") || backend.calls != 0 || s.Pending != nil {
			t.Fatal(result)
		}
	}
}

func TestSpokenAgendaRefusalClearsProposal(t *testing.T) {
	s, _ := multiSetup(t)
	args := json.RawMessage(`{"title":"pocket","start":"2026-09-18T10:00:00-03:00","end":"2026-09-18T11:00:00-03:00"}`)
	s.ObserveUser("u1", "Quero agendar pocket em dezoito de setembro às dez por uma hora")
	s.Plan("proposal", "local_agenda_create_event", "u1", args)
	if s.Pending == nil {
		t.Fatal("missing proposal")
	}
	s.ObserveUser("u2", "não")
	if s.Pending != nil || s.ObserveUser("u3", "Confirmo agendar") {
		t.Fatal("refused target approved")
	}
}
