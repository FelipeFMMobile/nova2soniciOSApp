package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/protocol"
)

func rawEvent(c *websocket.Conn, kind string, value any) error {
	return c.WriteJSON(map[string]any{"type": "nova.event", "payload": map[string]any{"event": map[string]any{kind: value}}})
}
func bridge(t *testing.T, handler func(*websocket.Conn, map[string]any)) string {
	t.Helper()
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		var start map[string]any
		if c.ReadJSON(&start) != nil {
			return
		}
		_ = rawEvent(c, "usageEvent", map[string]any{})
		_ = c.WriteJSON(map[string]string{"type": "session.ready"})
		handler(c, start)
	}))
	t.Cleanup(h.Close)
	return "ws" + strings.TrimPrefix(h.URL, "http")
}

func nativeUser(c *websocket.Conn, id string) {
	_ = rawEvent(c, "contentStart", map[string]any{"contentId": id, "type": "TEXT", "role": "USER", "additionalModelFields": `{"generationStage":"FINAL"}`})
	_ = rawEvent(c, "textOutput", map[string]any{"contentId": id, "content": "Pergunta em português"})
	_ = rawEvent(c, "contentEnd", map[string]any{"contentId": id, "type": "TEXT", "stopReason": "END_TURN"})
}
func nativeAudio(c *websocket.Conn, id string) {
	_ = rawEvent(c, "contentStart", map[string]any{"contentId": id, "type": "AUDIO", "role": "ASSISTANT", "audioOutputConfiguration": map[string]any{"sampleRateHertz": 24000}})
	_ = rawEvent(c, "audioOutput", map[string]any{"contentId": id, "content": "AAAAAA=="})
}
func startNova(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	send(t, c, protocol.Event{Type: protocol.SessionStart, Provider: "nova"})
	return until(t, c, protocol.SessionReady).SessionID
}

func TestNovaForwardsBeforeCommitAndUsesNativeBargeIn(t *testing.T) {
	seen := make(chan struct{}, 1)
	url := bridge(t, func(c *websocket.Conn, _ map[string]any) {
		var audio map[string]any
		if c.ReadJSON(&audio) != nil {
			return
		}
		seen <- struct{}{}
		nativeUser(c, "u-a")
		nativeAudio(c, "a-a")
		if c.ReadJSON(&audio) != nil {
			return
		}
		_ = rawEvent(c, "textOutput", map[string]any{"contentId": "a-a", "content": `{"interrupted":true}`})
		nativeUser(c, "u-b")
		nativeAudio(c, "a-b")
		_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "a-b", "type": "AUDIO", "stopReason": "END_TURN"})
		for c.ReadJSON(&audio) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.NovaBridgeURL = url
	_, _, gatewayURL := setup(t, cfg)
	c := dial(t, gatewayURL, "")
	sid := startNova(t, c)
	appendAudio(t, c, sid, "input", 1)
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("input was buffered until commit")
	}
	first := until(t, c, protocol.AudioOutput)
	if first.TurnID != "input" || first.Sequence != 1 {
		t.Fatal(first)
	}
	// Same microphone container stays live; interruption comes from Nova, not ID change.
	appendAudio(t, c, sid, "input", 2)
	if e := until(t, c, protocol.TurnInterrupted); e.TurnID != first.TurnID {
		t.Fatal(e)
	}
	second := until(t, c, protocol.AudioOutput)
	if second.TurnID == first.TurnID || second.Sequence != 1 {
		t.Fatal("native turn did not advance", second)
	}
	until(t, c, protocol.TurnCompleted)
	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
	until(t, c, protocol.SessionStopped)
}

func TestNovaRenewalReplaysFinalHistory(t *testing.T) {
	var connections atomic.Int64
	histories := make(chan []any, 8)
	url := bridge(t, func(c *websocket.Conn, start map[string]any) {
		number := connections.Add(1)
		if history, ok := start["history"].([]any); ok {
			histories <- history
		}
		var input map[string]any
		if number == 1 {
			if c.ReadJSON(&input) != nil {
				return
			}
			nativeUser(c, "u")
			nativeAudio(c, "a")
			_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "a", "type": "AUDIO", "stopReason": "END_TURN"})
			_ = rawEvent(c, "contentStart", map[string]any{"contentId": "final", "type": "TEXT", "role": "ASSISTANT", "additionalModelFields": `{"generationStage":"FINAL"}`})
			_ = rawEvent(c, "textOutput", map[string]any{"contentId": "final", "content": "Resposta realmente falada"})
			_ = rawEvent(c, "contentEnd", map[string]any{"contentId": "final", "type": "TEXT", "stopReason": "END_TURN"})
		}
		for c.ReadJSON(&input) == nil {
		}
	})
	cfg := testConfig()
	cfg.Provider = "nova"
	cfg.NovaBridgeURL = url
	cfg.NovaMaxSessionAge = 300 * time.Millisecond
	_, _, gatewayURL := setup(t, cfg)
	c := dial(t, gatewayURL, "")
	sid := startNova(t, c)
	appendAudio(t, c, sid, "input", 1)
	until(t, c, protocol.TurnCompleted)
	until(t, c, protocol.SessionRenewed)
	select {
	case history := <-histories:
		if len(history) != 2 {
			t.Fatal("history lost", history)
		}
	case <-time.After(time.Second):
		t.Fatal("renewal did not replay history")
	}
	send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
	until(t, c, protocol.SessionStopped)
}

func TestNovaCancelAndProviderFailure(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{true: "disconnect", false: "cancel"}[disconnect], func(t *testing.T) {
			var calls atomic.Int64
			url := bridge(t, func(c *websocket.Conn, _ map[string]any) {
				calls.Add(1)
				var input any
				if c.ReadJSON(&input) != nil {
					return
				}
				nativeUser(c, "u")
				nativeAudio(c, "a")
				if disconnect {
					return
				}
				for c.ReadJSON(&input) == nil {
				}
			})
			cfg := testConfig()
			cfg.Provider = "nova"
			cfg.NovaBridgeURL = url
			_, _, gatewayURL := setup(t, cfg)
			c := dial(t, gatewayURL, "")
			sid := startNova(t, c)
			appendAudio(t, c, sid, "input", 1)
			until(t, c, protocol.AudioOutput)
			if disconnect {
				if e := until(t, c, protocol.Error); e.Code != "provider_unavailable" {
					t.Fatal(e)
				}
				return
			}
			send(t, c, protocol.Event{Type: protocol.TurnCancel, SessionID: sid, TurnID: "input"})
			until(t, c, protocol.TurnInterrupted)
			until(t, c, protocol.SessionRenewed)
			deadline := time.Now().Add(time.Second)
			for calls.Load() < 2 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if calls.Load() != 2 {
				t.Fatal("cancel did not replace old generation")
			}
			send(t, c, protocol.Event{Type: protocol.SessionStop, SessionID: sid})
			until(t, c, protocol.SessionStopped)
		})
	}
}
