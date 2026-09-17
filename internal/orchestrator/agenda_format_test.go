package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"stsmodel.local/poc/internal/mcp"
)

func TestAgendaDirectCreationWithoutObservedUserTranscript(t *testing.T) {
	s, backend := multiSetup(t)
	args := json.RawMessage(`{"title":"reunião","start":"2026-09-18T10:00:00-03:00","end":"2026-09-18T11:00:00-03:00"}`)
	job, result := s.Plan("direct", "local_agenda_create_event", "u", args)
	if result != nil || s.Pending != nil {
		t.Fatal("must create without transcript or confirmation", result)
	}
	resultValue := s.Execute(context.Background(), job)
	s.Complete(job, resultValue)
	if resultValue.IsError || mcp.Status(resultValue) != "created" || backend.calls != 1 {
		t.Fatal(resultValue)
	}
	_, result = s.Plan("retry", "local_agenda_create_event", "u", args)
	if result == nil || result.IsError || backend.calls != 1 {
		t.Fatal("retry must not duplicate", result)
	}
}

func TestAgendaMalformedFormatIsRejectedButValidCallNeedsNoASRIntent(t *testing.T) {
	s, backend := multiSetup(t)
	s.ObserveUser("u", "Talvez agende reunião dia 18 de setembro de 2026 às dez por uma hora")
	bad := json.RawMessage(`{"title":"reunião","start":"18 de setembro de 2026 às 10 horas","end":"2026-09-18 11:00"}`)
	_, result := s.Plan("bad", "local_agenda_create_event", "u", bad)
	if result == nil || !result.IsError || !strings.Contains(result.Content[0].Text, "invalid_datetime_format") || !strings.Contains(result.Content[0].Text, "expected_format") {
		t.Fatal(result)
	}
	valid := json.RawMessage(`{"title":"reunião","start":"2026-09-18T10:00:00-03:00","end":"2026-09-18T11:00:00-03:00"}`)
	_, result = s.Plan("fixed", "local_agenda_create_event", "u", valid)
	if result != nil {
		t.Fatal("valid call must not require ASR intent", result)
	}
	if s.Pending != nil || backend.calls != 0 {
		t.Fatal("planning must not execute or request confirmation")
	}
}

func TestAgendaFormatValidationAcceptsRFC3339AndRejectsAmbiguity(t *testing.T) {
	for _, value := range []string{"2026-09-18T10:00:00-03:00", "2026-09-18T13:00:00Z"} {
		args, _ := json.Marshal(map[string]string{"start": value, "end": value})
		if result := agendaFormatError(args); result != nil {
			t.Fatal(value, result)
		}
	}
	for _, value := range []any{"18/09/2026", "2026-09-18T10:00:00", "2026-09-18T10:00:00.5-03:00", "2026-09-18T10:00:00.000-03:00", "2026-02-30T10:00:00-03:00", 123, nil} {
		args, _ := json.Marshal(map[string]any{"start": value, "end": "2026-09-18T11:00:00-03:00"})
		result := agendaFormatError(args)
		if result == nil || !result.IsError || mcp.Status(*result) != "error" {
			t.Fatal(value, result)
		}
	}
}
