package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestDurableRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	args := json.RawMessage(`{"title":"Código","content":"valor exclusivo 7419"}`)
	first, err := s.Mutate(ctx, "request-1", "notes.create", args)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	retry, err := s.Mutate(ctx, "request-1", "notes.create", args)
	if err != nil || string(first) != string(retry) {
		t.Fatal("retry did not replay", err)
	}
	if _, err = s.Mutate(ctx, "request-1", "notes.create", json.RawMessage(`{"title":"Outro","content":"outro"}`)); err == nil {
		t.Fatal("conflicting retry accepted")
	}
	notes, err := s.List(ctx, "7419")
	if err != nil || len(notes) != 1 {
		t.Fatal(notes, err)
	}
	deletion, _ := json.Marshal(map[string]string{"id": notes[0].ID})
	if _, err = s.Mutate(ctx, "delete-1", "notes.delete", deletion); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Mutate(ctx, "delete-1", "notes.delete", deletion); err != nil {
		t.Fatal(err)
	}
	notes, err = s.List(ctx, "")
	if err != nil || len(notes) != 0 {
		t.Fatal(notes, err)
	}
}

func TestSessionAuditKeepsUncertainOutcomes(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "audit.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err = s.StartSession(ctx, "session-1", "request-1"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordTurn(ctx, "session-1", "turn-1", "turn.started"); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"running", "confirmation_required", "completed"} {
		if err = s.RecordTool(ctx, "session-1", state, "turn-1", "notes.delete", "retry-key", state); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.EndSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"running": "outcome_unknown", "confirmation_required": "cancelled", "completed": "completed"} {
		var got string
		err = s.db.QueryRow(`SELECT state FROM tool_operations WHERE id=?`, id).Scan(&got)
		if err != nil || got != want {
			t.Fatal(id, got, err)
		}
	}
}
