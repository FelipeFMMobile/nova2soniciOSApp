package nova

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBridgeHandshakeAndBackpressureClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var start map[string]any
		if c.ReadJSON(&start) != nil {
			return
		}
		_ = c.SetWriteDeadline(time.Now().Add(time.Second))
		_ = c.WriteJSON(map[string]any{"type": "nova.event", "payload": map[string]any{"event": map[string]any{"usageEvent": map[string]any{}}}})
		_ = c.WriteJSON(map[string]string{"type": "session.ready"})
		for i := 0; i < 100; i++ {
			if c.WriteJSON(map[string]any{"type": "nova.event", "payload": map[string]any{"event": map[string]any{"usageEvent": map[string]any{}}}}) != nil {
				return
			}
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := Open(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked reader leaked")
	}
}

func TestBridgeStartupRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var start any
		_ = c.ReadJSON(&start)
		_ = c.WriteJSON(map[string]string{"type": "error", "message": "secret details must not reach clients"})
	}))
	defer server.Close()
	if _, err := Open(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
