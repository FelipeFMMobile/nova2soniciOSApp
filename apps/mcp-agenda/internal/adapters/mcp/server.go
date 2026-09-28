package adapter

import (
	"context"
	"encoding/json"
	"io"
	"stsmodel.local/mcp-agenda/contracts/v1"
	storage "stsmodel.local/mcp-agenda/internal/scheduling"
	. "stsmodel.local/mcpwire"
)

func ServeAgenda(ctx context.Context, in io.Reader, out io.Writer, s Repository, envelope ...*EnvelopeCodec) error {
	return ServeTools(ctx, in, out, "sts-agenda", contracts.Tools(), func(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) Result {
		var i storage.AgendaInput
		_ = json.Unmarshal(args, &i)
		switch name {
		case "agenda.list_events":
			events, err := s.Events(ctx, i.Start, i.End)
			if err != nil {
				return Failure("invalid_interval_or_storage")
			}
			return TextResult(map[string]any{"status": "ok", "events": events, "timezone": storage.AgendaTimezone}, false)
		case "agenda.list_slots":
			slots, err := s.Slots(ctx, i)
			if err != nil {
				return Failure("invalid_interval_or_storage")
			}
			return TextResult(map[string]any{"status": "ok", "slots": slots, "timezone": storage.AgendaTimezone}, false)
		default:
			data, err := s.MutateAgenda(ctx, key, name, args, confirmed)
			if err != nil {
				code := err.Error()
				switch code {
				case "invalid_interval", "outside_availability", "slot_conflict", "event_not_found", "confirmation_required", "idempotency_conflict", "invalid_title", "invalid_key":
				default:
					code = "mutation_failed"
				}
				return Failure(code)
			}
			return TextResult(data, false)
		}
	}, envelope...)
}

type Repository interface {
	Events(context.Context, string, string) ([]storage.AgendaEvent, error)
	Slots(context.Context, storage.AgendaInput) ([]storage.AgendaEvent, error)
	MutateAgenda(context.Context, string, string, json.RawMessage, bool) (json.RawMessage, error)
}
