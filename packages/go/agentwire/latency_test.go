package agentwire

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

type latencyFactory struct{}
type latencySession struct{ events chan AgentEvent }

func (latencyFactory) Open(context.Context, AgentSessionStart) (AgentSession, error) {
	return &latencySession{make(chan AgentEvent, RemoteQueueDepth)}, nil
}
func (s *latencySession) Send(ctx context.Context, c AgentCommand) error {
	select {
	case s.events <- AgentEvent{Type: OutputAudio, TurnID: c.TurnID, Sequence: c.Sequence, SampleRate: 24000, Audio: make([]byte, 960)}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *latencySession) Events() <-chan AgentEvent { return s.events }
func (s *latencySession) Close() error              { return nil }

// Opt-in because host load is not a deterministic unit-test assertion. Uses
// 20 ms PCM16 input/output frames, warmup and 1/8 simultaneous sessions.
// Measures send-to-first-output; connection establishment and model are excluded.
func TestLocalTransportLatency(t *testing.T) {
	if os.Getenv("STS_AGENT_LATENCY") != "1" {
		t.Skip("opt-in latency gate")
	}
	h, err := NewHandler(ServerConfig{AgentID: "latency", Token: "benchmark-token", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: 30 * time.Second, MaxSessions: 16}, latencyFactory{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	remote, err := NewRemoteFactory([]Endpoint{endpointFor(server, "latency", "benchmark-token")})
	if err != nil {
		t.Fatal(err)
	}
	for _, concurrency := range []int{1, 8} {
		results := map[string][]float64{}
		for _, mode := range []string{"inprocess", "remote"} {
			var factory AgentSessionFactory = latencyFactory{}
			if mode == "remote" {
				factory = remote
			}
			samples := make(chan float64, concurrency*1000)
			failures := make(chan error, concurrency)
			var wg sync.WaitGroup
			for worker := 0; worker < concurrency; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					sid := fmt.Sprintf("latency-%d", worker)
					session, err := factory.Open(ctx, AgentSessionStart{AgentID: "latency", SessionID: sid, Provider: "fake", Principal: Principal{Subject: "benchmark-user"}})
					if err != nil {
						failures <- err
						return
					}
					defer session.Close()
					for i := 0; i < 1100; i++ {
						at := time.Now()
						if err := session.Send(ctx, AgentCommand{Type: InputAudio, SessionID: sid, TurnID: "turn", Sequence: uint64(i + 1), Audio: make([]byte, 640)}); err != nil {
							failures <- err
							return
						}
						select {
						case event, ok := <-session.Events():
							if !ok || event.Type != OutputAudio || event.Sequence != uint64(i+1) {
								failures <- fmt.Errorf("unexpected output")
								return
							}
						case <-ctx.Done():
							failures <- ctx.Err()
							return
						}
						if i >= 100 {
							samples <- float64(time.Since(at).Nanoseconds()) / 1e6
						}
					}
				}(worker)
			}
			wg.Wait()
			close(samples)
			close(failures)
			for err := range failures {
				t.Fatal(err)
			}
			for sample := range samples {
				results[mode] = append(results[mode], sample)
			}
			sort.Float64s(results[mode])
		}
		percentile := func(mode string, p int) float64 { v := results[mode]; return v[(len(v)*p+99)/100-1] }
		delta := percentile("remote", 95) - percentile("inprocess", 95)
		t.Logf("sessions=%d samples=%d frame_in=640 frame_out=960 inprocess_p50_ms=%.4f inprocess_p95_ms=%.4f remote_p50_ms=%.4f remote_p95_ms=%.4f delta_p95_ms=%.4f budget_ms=10", concurrency, len(results["remote"]), percentile("inprocess", 50), percentile("inprocess", 95), percentile("remote", 50), percentile("remote", 95), delta)
		if delta > 10 {
			t.Fatalf("internal hop exceeds 10 ms p95: %.4f", delta)
		}
	}
}
