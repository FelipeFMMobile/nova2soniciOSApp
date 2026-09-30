package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/agentwire"
	"stsmodel.local/voice-gateway/internal/bootstrap/config"
	"stsmodel.local/voicewire"
)

type Counters struct {
	Sessions         atomic.Int64
	Completed        atomic.Int64
	Interrupted      atomic.Int64
	Errors           atomic.Int64
	FirstAudioCount  atomic.Int64
	FirstAudioMicros atomic.Int64
	firstAudioWindow latencyWindow
}

// latencyWindow bounds metric memory while retaining enough recent samples to
// characterize the extra hop introduced by a future remote agent session.
type latencyWindow struct {
	mu     sync.Mutex
	values []int64
	next   int
}

const maxLatencySamples = 1024

func (w *latencyWindow) add(micros int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.values) < maxLatencySamples {
		w.values = append(w.values, micros)
		return
	}
	w.values[w.next] = micros
	w.next = (w.next + 1) % len(w.values)
}

func (w *latencyWindow) percentiles() (float64, float64) {
	w.mu.Lock()
	values := append([]int64(nil), w.values...)
	w.mu.Unlock()
	if len(values) == 0 {
		return 0, 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	value := func(percent int) float64 {
		index := (percent*len(values) + 99) / 100
		if index < 1 {
			index = 1
		}
		return float64(values[index-1]) / 1000
	}
	return value(50), value(95)
}

func (c *Counters) recordFirstAudio(duration time.Duration) {
	c.FirstAudioCount.Add(1)
	c.FirstAudioMicros.Add(duration.Microseconds())
	c.firstAudioWindow.add(duration.Microseconds())
}

type Server struct {
	cfg          config.Config
	logger       *slog.Logger
	mux          *http.ServeMux
	ctx          context.Context
	cancel       context.CancelFunc
	slots        chan struct{}
	mu           sync.Mutex
	connections  map[*websocket.Conn]struct{}
	wg           sync.WaitGroup
	metrics      Counters
	agentFactory agentwire.AgentSessionFactory
}

type principalContextKey struct{}

func New(cfg config.Config, logger *slog.Logger) (*Server, error) {
	if cfg.DefaultAgentID == "" {
		cfg.DefaultAgentID = "personal-assistant"
	}
	if err := agentwire.ValidatePrincipal(cfg.Principal); err != nil {
		return nil, err
	}
	if cfg.Provider != "fake" && cfg.Provider != "nova" {
		return nil, fmt.Errorf("unsupported provider")
	}
	if cfg.MaxSessions <= 0 || cfg.MaxEventBytes <= 0 || cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout < time.Millisecond {
		return nil, fmt.Errorf("gateway limits and timeouts must be positive")
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{cfg: cfg, logger: logger, mux: http.NewServeMux(), ctx: ctx, cancel: cancel, slots: make(chan struct{}, cfg.MaxSessions), connections: make(map[*websocket.Conn]struct{})}
	factory, err := agentwire.NewRemoteFactory(cfg.AgentCatalog)
	if err != nil {
		cancel()
		return nil, err
	}
	s.agentFactory = factory
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
		ctx := context.WithValue(r.Context(), principalContextKey{}, s.cfg.Principal)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"selected": s.cfg.Provider, "agentMode": "remote", "providers": []map[string]any{
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
	p50, p95 := s.metrics.firstAudioWindow.percentiles()
	writeJSON(w, http.StatusOK, map[string]any{"activeSessions": len(s.slots), "totalSessions": s.metrics.Sessions.Load(), "completedTurns": s.metrics.Completed.Load(), "interruptedTurns": s.metrics.Interrupted.Load(), "errors": s.metrics.Errors.Load(), "firstAudioSamples": count, "firstAudioMeanMs": mean, "firstAudioP50Ms": p50, "firstAudioP95Ms": p95})
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

type inbound struct {
	event protocol.Event
	err   error
}
