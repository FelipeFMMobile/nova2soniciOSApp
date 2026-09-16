package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"stsmodel.local/poc/internal/storage"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve never writes logs to stdout. Host-only metadata carries retry identity
// and approval: neither is accepted as a model tool argument.
func Serve(ctx context.Context, in io.Reader, out io.Writer, store *storage.Store) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	enc := json.NewEncoder(out)
	initialized, ready := false, false
	tools := NotesTools()
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var req rpcMessage
		if json.Unmarshal(scanner.Bytes(), &req) != nil || req.JSONRPC != "2.0" {
			if err := enc.Encode(rpcMessage{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "invalid JSON-RPC"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			if req.Method == "notifications/initialized" && initialized {
				ready = true
			}
			continue
		}
		resp := rpcMessage{JSONRPC: "2.0", ID: req.ID}
		var result any
		switch {
		case req.Method == "initialize" && !initialized:
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(req.Params, &p) != nil || p.ProtocolVersion != ProtocolVersion {
				resp.Error = &rpcError{-32602, "unsupported protocol version"}
				break
			}
			initialized = true
			result = map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "sts-notes", "version": "0.5.0"}}
		case req.Method == "ping":
			result = map[string]any{}
		case !ready:
			resp.Error = &rpcError{-32000, "initialization required"}
		case req.Method == "tools/list":
			result = map[string]any{"tools": tools}
		case req.Method == "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
				Meta      struct {
					Key       string `json:"sts/idempotencyKey"`
					Confirmed bool   `json:"sts/confirmed"`
				} `json:"_meta"`
			}
			if json.Unmarshal(req.Params, &p) != nil {
				resp.Error = &rpcError{-32602, "invalid call"}
				break
			}
			var selected *Tool
			for i := range tools {
				if tools[i].Name == p.Name {
					selected = &tools[i]
					break
				}
			}
			if selected == nil {
				resp.Error = &rpcError{-32602, "unknown tool"}
				break
			}
			schema, err := Compile(selected.InputSchema)
			if err != nil || Validate(schema, p.Arguments) != nil {
				result = Failure("invalid_arguments")
				break
			}
			if p.Name == "notes.list" {
				var args struct {
					Query string `json:"query"`
				}
				_ = json.Unmarshal(p.Arguments, &args)
				notes, err := store.List(ctx, args.Query)
				if err != nil {
					result = Failure("storage_failed")
				} else {
					result = TextResult(map[string]any{"status": "ok", "notes": notes}, false)
				}
			} else {
				if p.Name == "notes.delete" && !p.Meta.Confirmed {
					result = Failure("confirmation_required")
					break
				}
				data, err := store.Mutate(ctx, p.Meta.Key, p.Name, p.Arguments)
				if err != nil {
					result = Failure("mutation_failed")
				} else {
					result = TextResult(data, false)
				}
			}
		default:
			resp.Error = &rpcError{-32601, "method not found"}
		}
		if resp.Error == nil {
			resp.Result, _ = json.Marshal(result)
			if len(resp.Result) > 65536 {
				resp.Result, _ = json.Marshal(Failure("result_too_large"))
			}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}
