package provider

import (
	"context"

	"stsmodel.local/poc/internal/protocol"
)

// VoiceProvider streams one turn. Respond must return promptly on cancellation.
// Stage 3 will add a continuous Nova session adapter behind this boundary.
type VoiceProvider interface {
	ID() string
	Respond(context.Context, []byte, func(protocol.Event) error) error
}
