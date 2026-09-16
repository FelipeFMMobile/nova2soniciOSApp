package provider

import (
	"context"

	"stsmodel.local/poc/internal/protocol"
)

// VoiceProvider streams one turn. Respond must return promptly on cancellation.
// The fake path uses this turn boundary. Continuous Nova input uses nova.Stream
// so microphone frames remain live during response generation.
type VoiceProvider interface {
	ID() string
	Respond(context.Context, []byte, func(protocol.Event) error) error
}
