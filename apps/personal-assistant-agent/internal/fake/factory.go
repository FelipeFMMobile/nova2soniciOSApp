package fake

import (
	"log/slog"
	"stsmodel.local/agentwire"
	"stsmodel.local/personal-assistant-agent/internal/fake/provider"
	"time"
)

type config struct{ TurnTimeout time.Duration }
type Server struct {
	cfg    config
	logger *slog.Logger
	voice  provider.Fake
}

func New(timeout time.Duration, logger *slog.Logger) agentwire.AgentSessionFactory {
	return factory{server: &Server{cfg: config{timeout}, logger: logger}}
}
