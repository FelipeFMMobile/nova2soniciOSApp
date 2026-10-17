package orchestrator

import (
	"context"
	"encoding/json"
	shared "stsmodel.local/actionruntime"
	"stsmodel.local/mcpwire"
	"stsmodel.local/personal-assistant-agent/config/integrations"
)

type Backend = shared.Backend
type Session = shared.Session
type Pending = shared.Pending
type Job = shared.Job

func New(ctx context.Context, backend Backend, allowed []string, namespace string) (*Session, error) {
	profiles, err := integrations.Load()
	if err != nil {
		return nil, err
	}
	return shared.New(ctx, backend, allowed, namespace, profiles)
}
func datetimeFormatError(args json.RawMessage, fields []string) *mcp.Result {
	return shared.DateTimeFormatError(args, fields)
}
