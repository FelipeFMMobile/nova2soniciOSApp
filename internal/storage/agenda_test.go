package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
)

var agendaArgs = json.RawMessage(`{"title":"Fictício","start":"2030-05-20T09:00:00-03:00","end":"2030-05-20T10:00:00-03:00"}`)

func TestAgendaUTCConflictReplayAndCancel(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agenda.sqlite")
	s, err := OpenAgenda(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.MutateAgenda(ctx, "request/tool", "agenda.create_event", agendaArgs, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenAgenda(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replay, err := s.MutateAgenda(ctx, "request/tool", "agenda.create_event", agendaArgs, false)
	if err != nil || string(replay) != string(data) {
		t.Fatal("durable replay", err)
	}
	if _, err = s.MutateAgenda(ctx, "other", "agenda.create_event", agendaArgs, false); err == nil {
		t.Fatal("conflict accepted")
	}
	if _, err = s.MutateAgenda(ctx, "request/tool", "agenda.create_event", json.RawMessage(`{"title":"changed"}`), false); err == nil {
		t.Fatal("key conflict accepted")
	}
	events, err := s.Events(ctx, "2030-05-20T00:00:00Z", "2030-05-21T00:00:00Z")
	if err != nil || len(events) != 1 || events[0].Start != "2030-05-20T12:00:00Z" {
		t.Fatal(events, err)
	}
	args, _ := json.Marshal(map[string]string{"id": events[0].ID})
	if _, err = s.MutateAgenda(ctx, "cancel", "agenda.cancel_event", args, false); err == nil {
		t.Fatal("unconfirmed cancellation")
	}
	if _, err = s.MutateAgenda(ctx, "cancel", "agenda.cancel_event", args, true); err != nil {
		t.Fatal(err)
	}
	slots, err := s.Slots(ctx, AgendaInput{Start: "2030-05-20T09:00:00-03:00", End: "2030-05-20T10:00:00-03:00", Duration: 60})
	if err != nil || len(slots) != 1 {
		t.Fatal(slots, err)
	}
}
func TestAgendaIntervalsAndFixtures(t *testing.T) {
	ctx := context.Background()
	s, err := OpenAgenda(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, pair := range [][2]string{{"2030-05-20T09:00:00", "2030-05-20T10:00:00"}, {"2030-05-20T10:00:00Z", "2030-05-20T09:00:00Z"}, {"2030-01-01T00:00:00Z", "2030-12-31T00:00:00Z"}} {
		if _, err = s.Events(ctx, pair[0], pair[1]); err == nil {
			t.Fatal("invalid interval", pair)
		}
	}
	for _, start := range []string{"2030-05-18T09:00:00-03:00", "2030-05-20T08:00:00-03:00"} {
		a := json.RawMessage(`{"title":"x","start":"` + start + `","end":"2030-05-20T10:00:00-03:00"}`)
		if _, err = s.MutateAgenda(ctx, start, "agenda.create_event", a, false); err == nil {
			t.Fatal("outside availability")
		}
	}
	for n := 0; n < 2; n++ {
		if err = s.SeedFixtures(ctx); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.Events(ctx, "2030-05-16T00:00:00Z", "2030-05-17T00:00:00Z")
	if err != nil || len(events) != 1 || events[0].ID != "fixture-2030" {
		t.Fatal(events, err)
	}
}
func TestAgendaConcurrentConflictIsAtomic(t *testing.T) {
	s, err := OpenAgenda(filepath.Join(t.TempDir(), "agenda.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"a", "b"} {
		wg.Go(func() {
			_, err := s.MutateAgenda(context.Background(), key, "agenda.create_event", agendaArgs, false)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal(successes)
	}
}
