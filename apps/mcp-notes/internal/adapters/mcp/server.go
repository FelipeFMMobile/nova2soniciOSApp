package adapter

import (
	"context"
	"encoding/json"
	"io"
	"stsmodel.local/mcp-notes/contracts/v1"
	"stsmodel.local/mcp-notes/internal/notes"
	. "stsmodel.local/mcpwire"
)

// Serve never writes logs to stdout. Host-only metadata carries retry identity
// and approval: neither is accepted as a model tool argument.
func Serve(ctx context.Context, in io.Reader, out io.Writer, store Repository, envelope ...*EnvelopeCodec) error {
	return ServeTools(ctx, in, out, "sts-notes", contracts.Tools(), func(ctx context.Context, name string, args json.RawMessage, key string, confirmed bool) Result {
		if name == "notes.list" {
			var input struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal(args, &input)
			notes, err := store.List(ctx, input.Query)
			if err != nil {
				return Failure("storage_failed")
			}
			return TextResult(map[string]any{"status": "ok", "notes": notes}, false)
		}
		if name == "notes.delete" && !confirmed {
			return Failure("confirmation_required")
		}
		data, err := store.Mutate(ctx, key, name, args)
		if err != nil {
			return Failure("mutation_failed")
		}
		return TextResult(data, false)
	}, envelope...)
}

// Repository is the application capability required by this adapter.
type Repository interface {
	List(context.Context, string) ([]notes.Note, error)
	Mutate(context.Context, string, string, json.RawMessage) (json.RawMessage, error)
}
