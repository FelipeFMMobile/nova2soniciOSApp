package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/config"
	"stsmodel.local/poc/internal/orchestrator"
	"stsmodel.local/poc/internal/provider"
)

type Counters struct {
	Sessions         atomic.Int64
	Completed        atomic.Int64
	Interrupted      atomic.Int64
	Errors           atomic.Int64
	FirstAudioCount  atomic.Int64
	FirstAudioMicros atomic.Int64
}

type Server struct {
	cfg         config.Config
	logger      *slog.Logger
	voice       provider.VoiceProvider
	mux         *http.ServeMux
	ctx         context.Context
	cancel      context.CancelFunc
	slots       chan struct{}
	mu          sync.Mutex
	connections map[*websocket.Conn]struct{}
	wg          sync.WaitGroup
	metrics     Counters
	toolFactory func(context.Context) (orchestrator.Backend, func(), error)
}

func New(cfg config.Config, logger *slog.Logger) (*Server, error) {
	if cfg.Provider != "fake" && cfg.Provider != "nova" {
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	if cfg.NovaMaxSessionAge == 0 {
		cfg.NovaMaxSessionAge = 7*time.Minute + 30*time.Second
	}
	if cfg.MCPTimeout == 0 {
		cfg.MCPTimeout = 10 * time.Second
	}
	if cfg.MCPTimeout <= 0 {
		return nil, fmt.Errorf("MCP timeout must be positive")
	}
	if cfg.Provider == "nova" && (cfg.LiteLLMURL == "" || cfg.LiteLLMAPIKey == "" || cfg.LiteLLMModel == "" || cfg.NovaMaxSessionAge <= 0) {
		return nil, fmt.Errorf("Nova requires LiteLLM configuration and positive session age")
	}
	if cfg.MaxSessions <= 0 || cfg.MaxEventBytes <= 0 || cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout < time.Millisecond || cfg.TurnTimeout <= 0 {
		return nil, fmt.Errorf("gateway limits and timeouts must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{cfg: cfg, logger: logger, voice: provider.Fake{}, mux: http.NewServeMux(), ctx: ctx, cancel: cancel, slots: make(chan struct{}, cfg.MaxSessions), connections: make(map[*websocket.Conn]struct{})}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "protocolVersion": 1})
	})
	s.mux.HandleFunc("GET /v1/providers", s.authorize(s.providers))
	s.mux.HandleFunc("GET /v1/voice", s.authorize(s.handleVoice))
	s.mux.HandleFunc("GET /v1/metrics", s.authorize(s.statistics))
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.ctx.Err() != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway shutting down"})
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.DevelopmentToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.cfg.DevelopmentToken)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"selected": s.cfg.Provider, "providers": []map[string]any{
		{"id": "fake", "available": true, "speech": false, "inputSampleRate": 16000, "outputSampleRate": 24000},
		{"id": "nova", "available": true, "speech": true, "inputSampleRate": 16000, "outputSampleRate": 24000, "requires": "LiteLLM Realtime and AWS Bedrock authorization"},
	}})
}

func (s *Server) statistics(w http.ResponseWriter, r *http.Request) {
	count := s.metrics.FirstAudioCount.Load()
	mean := float64(0)
	if count > 0 {
		mean = float64(s.metrics.FirstAudioMicros.Load()) / float64(count) / 1000
	}
	writeJSON(w, http.StatusOK, map[string]any{"activeSessions": len(s.slots), "totalSessions": s.metrics.Sessions.Load(), "completedTurns": s.metrics.Completed.Load(), "interruptedTurns": s.metrics.Interrupted.Load(), "errors": s.metrics.Errors.Load(), "firstAudioSamples": count, "firstAudioMeanMs": mean})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Close cancels providers and closes hijacked WebSockets before waiting.
// http.Server.Shutdown alone does not close hijacked connections.
func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	s.cancel()
	for connection := range s.connections {
		_ = connection.Close()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
