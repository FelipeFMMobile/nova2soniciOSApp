package provider

import (
	"context"
	"encoding/json"

	"stsmodel.local/poc/internal/protocol"
)

// ToolSpec is provider-neutral function metadata. Provider adapters translate
// it to their wire format; business policy remains in the orchestrator.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// VoiceProvider streams one turn. Respond must return promptly on cancellation.
// The fake path uses this turn boundary. Continuous Nova input uses nova.Stream
// so microphone frames remain live during response generation.
type VoiceProvider interface {
	ID() string
	Respond(context.Context, []byte, func(protocol.Event) error) error
}
