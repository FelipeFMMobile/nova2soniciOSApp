package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/config"
	"stsmodel.local/poc/internal/protocol"
)

func testConfig() config.Config {
	return config.Config{Environment: "development", Provider: "fake", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: 3 * time.Second, TurnTimeout: 2 * time.Second, MaxSessions: 16, MaxEventBytes: 512 * 1024}
}

func setup(t *testing.T, cfg config.Config) (*Server, *httptest.Server, string) {
	t.Helper()
	s, err := New(cfg, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
		h.Close()
	})
	return s, h, "ws" + strings.TrimPrefix(h.URL, "http") + "/v1/voice"
}

func dial(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	c, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func send(t *testing.T, c *websocket.Conn, e protocol.Event) {
	t.Helper()
	e.Version = 1
	if err := c.WriteJSON(e); err != nil {
		t.Fatal(err)
	}
}
func receive(t *testing.T, c *websocket.Conn) protocol.Event {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var e protocol.Event
	if err := c.ReadJSON(&e); err != nil {
		t.Fatal(err)
	}
	if e.Version != 1 {
		t.Fatal("wrong version")
	}
	return e
}
func until(t *testing.T, c *websocket.Conn, kind string) protocol.Event {
	t.Helper()
	for i := 0; i < 256; i++ {
		e := receive(t, c)
		if e.Type == kind {
			return e
		}
		if e.Type == protocol.Error {
			t.Fatalf("unexpected error: %+v", e)
		}
	}
	t.Fatal("missing event", kind)
	return protocol.Event{}
}
func start(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "fake"})
	return until(t, c, protocol.SessionReady).SessionID
}
func appendAudio(t *testing.T, c *websocket.Conn, sid, tid string, sequence uint64) {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: tid, Sequence: sequence, SampleRate: 16000, Audio: "AAAAAA=="})
}
func commit(t *testing.T, c *websocket.Conn, sid, tid string) {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.TurnCommit, SessionID: sid, TurnID: tid})
}

func TestHTTPAuthenticationAndOrigin(t *testing.T) {
	cfg := testConfig()
	cfg.DevelopmentToken = "test-secret"
	_, h, url := setup(t, cfg)
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"/healthz", "", 200}, {"/v1/providers", "", 401}, {"/v1/providers", "wrong", 401}, {"/v1/providers", "test-secret", 200}, {"/v1/metrics", "test-secret", 200}} {
		r, _ := http.NewRequest("GET", h.URL+tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("%s: %d", tc.path, resp.StatusCode)
		}
	}
	for _, tc := range []struct {
		headers http.Header
		status  int
	}{{http.Header{}, 401}, {http.Header{"Authorization": []string{"Bearer test-secret"}, "Origin": []string{"https://attacker.invalid"}}, 403}} {
		c, r, err := websocket.DefaultDialer.Dial(url, tc.headers)
		if c != nil {
			c.Close()
		}
		if err == nil || r == nil || r.StatusCode != tc.status {
			t.Fatal("unauthorized upgrade accepted", err)
		}
		r.Body.Close()
	}
	c := dial(t, url, "test-secret")
	sid := start(t, c)
	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
	until(t, c, protocol.SessionStopped)
}

func TestFakeConversationAudioAndMetrics(t *testing.T) {
	s, h, url := setup(t, testConfig())
	c := dial(t, url, "")
	sid := start(t, c)
	appendAudio(t, c, sid, "turn-a", 1)
	commit(t, c, sid, "turn-a")
	chunks := 0
	roles := map[string]bool{}
	for {
		e := receive(t, c)
		if e.Type == protocol.Error {
			t.Fatal(e)
		}
		if e.Type == protocol.Transcript {
			roles[e.Role] = true
		}
		if e.Type == protocol.AudioOutput {
			chunks++
			if e.SessionID != sid || e.TurnID != "turn-a" || e.Sequence != uint64(chunks) || e.SampleRate != 24000 {
				t.Fatal("audio contract mismatch", e)
			}
			pcm, err := base64.StdEncoding.DecodeString(e.Audio)
			if err != nil || len(pcm) != 1920 {
				t.Fatal("invalid pcm", err)
			}
		}
		if e.Type == protocol.TurnCompleted {
			if chunks != 20 || !roles["USER"] || !roles["ASSISTANT"] || e.Metrics == nil || e.Metrics.FirstAudioMS <= 0 || e.Metrics.DurationMS < e.Metrics.FirstAudioMS {
				t.Fatal("incomplete turn", chunks, e)
			}
			break
		}
	}
	if s.metrics.Completed.Load() != 1 {
		t.Fatal("missing completed metric")
	}
	resp, err := http.Get(h.URL + "/v1/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var stats map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats["firstAudioSamples"] != float64(1) {
		t.Fatal(stats)
	}
	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
	until(t, c, protocol.SessionStopped)
	var closed protocol.Event
	err = c.ReadJSON(&closed)
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatal("unclean close", err)
	}
}

func TestCancellationAndBargeInDiscardStaleAudio(t *testing.T) {
	for _, bargeIn := range []bool{false, true} {
		t.Run(fmt.Sprint(bargeIn), func(t *testing.T) {
			s, _, url := setup(t, testConfig())
			c := dial(t, url, "")
			sid := start(t, c)
			appendAudio(t, c, sid, "a", 1)
			commit(t, c, sid, "a")
			until(t, c, protocol.AudioOutput)
			if bargeIn {
				appendAudio(t, c, sid, "b", 1)
			} else {
				send(t, c, protocol.Event{Type: protocol.TurnCancel, SessionID: sid, TurnID: "a"})
			}
			e := until(t, c, protocol.TurnInterrupted)
			if e.TurnID != "a" || e.Metrics == nil {
				t.Fatal(e)
			}
			if !bargeIn {
				appendAudio(t, c, sid, "b", 1)
			}
			commit(t, c, sid, "b")
			chunks := uint64(0)
			for {
				e := receive(t, c)
				if e.Type == protocol.Error {
					t.Fatal(e)
				}
				if e.Type == protocol.AudioOutput {
					if e.TurnID != "b" {
						t.Fatal("stale audio after interruption", e)
					}
					chunks++
					if e.Sequence != chunks {
						t.Fatal("output sequence did not reset")
					}
				}
				if e.Type == protocol.TurnCompleted {
					if e.TurnID != "b" || chunks != 20 {
						t.Fatal(e)
					}
					break
				}
			}
			if s.metrics.Interrupted.Load() != 1 {
				t.Fatal("missing interruption metric")
			}
		})
	}
}

func TestInvalidEventsDoNotCorruptTurn(t *testing.T) {
	_, _, url := setup(t, testConfig())
	c := dial(t, url, "")
	sid := start(t, c)
	appendAudio(t, c, sid, "a", 1)
	until(t, c, protocol.TurnStarted)
	appendAudio(t, c, sid, "a", 1)
	if e := until(t, c, protocol.Error); e.Code != "invalid_audio" {
		t.Fatal(e)
	}
	send(t, c, protocol.Event{Type: protocol.TurnCommit, SessionID: "other", TurnID: "a"})
	if e := until(t, c, protocol.Error); e.Code != "session_mismatch" {
		t.Fatal(e)
	}
	if err := c.WriteMessage(websocket.BinaryMessage, []byte("binary")); err != nil {
		t.Fatal(err)
	}
	if e := until(t, c, protocol.Error); e.Code != "invalid_event" {
		t.Fatal(e)
	}
	appendAudio(t, c, sid, "a", 2)
	commit(t, c, sid, "a")
	until(t, c, protocol.TurnCompleted)
}

func TestLimitsAndReconnect(t *testing.T) {
	cfg := testConfig()
	cfg.MaxSessions = 1
	s, _, url := setup(t, cfg)
	first := dial(t, url, "")
	oldID := start(t, first)
	c, r, err := websocket.DefaultDialer.Dial(url, nil)
	if c != nil {
		c.Close()
	}
	if err == nil || r == nil || r.StatusCode != 503 {
		t.Fatal("session limit not enforced")
	}
	r.Body.Close()
	first.Close()
	deadline := time.Now().Add(time.Second)
	for len(s.slots) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(s.slots) != 0 {
		t.Fatal("session not released")
	}
	next := dial(t, url, "")
	if id := start(t, next); id == oldID {
		t.Fatal("connection resumed stale session")
	}
}

func TestOversizedMessageClosesConnection(t *testing.T) {
	cfg := testConfig()
	cfg.MaxEventBytes = 1024
	_, _, url := setup(t, cfg)
	c := dial(t, url, "")
	start(t, c)
	if err := c.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 2048))); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	var e protocol.Event
	if err := c.ReadJSON(&e); !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatal("oversized event not rejected", err)
	}
}

func TestProviderTimeoutAndSessionStartTimeout(t *testing.T) {
	cfg := testConfig()
	cfg.TurnTimeout = 10 * time.Millisecond
	_, _, url := setup(t, cfg)
	c := dial(t, url, "")
	sid := start(t, c)
	appendAudio(t, c, sid, "a", 1)
	commit(t, c, sid, "a")
	if e := until(t, c, protocol.Error); e.Code != "provider_timeout" {
		t.Fatal(e)
	}
	appendAudio(t, c, sid, "b", 1)
	until(t, c, protocol.TurnStarted)
	cfg.ReadTimeout = 30 * time.Millisecond
	_, _, otherURL := setup(t, cfg)
	other := dial(t, otherURL, "")
	_ = other.SetReadDeadline(time.Now().Add(time.Second))
	var e protocol.Event
	if err := other.ReadJSON(&e); err == nil {
		if e.Code != "session_timeout" {
			t.Fatal(e)
		}
	}
}

func TestConcurrentDisconnectsAndShutdownJoinWorkers(t *testing.T) {
	s, _, url := setup(t, testConfig())
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				failures <- err
				return
			}
			defer c.Close()
			_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
			if err := c.WriteJSON(protocol.Event{Version: 1, Type: protocol.SessionStart}); err != nil {
				failures <- err
				return
			}
			var e protocol.Event
			if err := c.ReadJSON(&e); err != nil {
				failures <- err
				return
			}
			sid := e.SessionID
			for _, event := range []protocol.Event{{Version: 1, Type: protocol.AudioAppend, SessionID: sid, TurnID: "a", Sequence: 1, SampleRate: 16000, Audio: "AAAAAA=="}, {Version: 1, Type: protocol.TurnCommit, SessionID: sid, TurnID: "a"}} {
				if err := c.WriteJSON(event); err != nil {
					failures <- err
					return
				}
			}
			for {
				if err := c.ReadJSON(&e); err != nil {
					failures <- err
					return
				}
				if e.Type == protocol.AudioOutput {
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	// Keep an additional live provider running while closing the whole gateway.
	c := dial(t, url, "")
	sid := start(t, c)
	appendAudio(t, c, sid, "active", 1)
	commit(t, c, sid, "active")
	until(t, c, protocol.AudioOutput)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal("workers did not join", err)
	}
	if len(s.slots) != 0 {
		t.Fatal("connections leaked")
	}
}

func TestUnavailableNovaIsNotSilentlyReplaced(t *testing.T) {
	cfg := testConfig()
	cfg.Provider = "nova"
	if _, err := New(cfg, nil); err == nil {
		t.Fatal("Nova silently fell back to fake")
	}
}
