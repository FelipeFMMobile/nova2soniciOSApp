package main

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"stsmodel.local/poc/internal/audio"
	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/protocol"
	"testing"
	"time"
)

func TestClientCapturesAndRequiresActualToolSuccess(t *testing.T) {
	for _, status := range []string{"created", "confirmation_required"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer c.Close()
				_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
				var start protocol.Event
				if c.ReadJSON(&start) != nil {
					return
				}
				if start.RequestID != "stable-request" {
					t.Error("request ID not forwarded")
				}
				emit := func(e protocol.Event) { e.Version = 1; e.SessionID = "session-1"; _ = c.WriteJSON(e) }
				emit(protocol.Event{Type: protocol.SessionReady, Provider: "fake"})
				var e protocol.Event
				for c.ReadJSON(&e) == nil {
					if e.Type == protocol.TurnCommit {
						break
					}
				}
				result := mcp.TextResult(map[string]string{"status": status}, false)
				data, _ := json.Marshal(result)
				emit(protocol.Event{Type: protocol.ToolResult, TurnID: e.TurnID, Tool: &protocol.Tool{OperationID: "tool-1", Name: "notes.create", Result: data}})
				emit(protocol.Event{Type: protocol.AudioOutput, TurnID: e.TurnID, Sequence: 1, SampleRate: 24000, Audio: "AAAAAA=="})
				emit(protocol.Event{Type: protocol.TurnCompleted, TurnID: e.TurnID})
				if c.ReadJSON(&e) == nil {
					emit(protocol.Event{Type: protocol.SessionStopped})
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			capture := filepath.Join(dir, "events.jsonl")
			err := runWithOptions(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), "", filepath.Join(dir, "audio.wav"), 0, "fake", "", clientOptions{RequestID: "stable-request", EventsPath: capture, ExpectedTools: []string{"notes.create"}})
			if (err == nil) != (status == "created") {
				t.Fatal("pending confirmation mistaken for success", err)
			}
			events, err := os.ReadFile(capture)
			if err != nil || !strings.Contains(string(events), "tool.result") {
				t.Fatal("events not captured", err)
			}
		})
	}
}

func TestClientFollowupAfterFirstResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(4 * time.Second))
		var e protocol.Event
		if c.ReadJSON(&e) != nil {
			return
		}
		emit := func(event protocol.Event) { event.Version = 1; event.SessionID = "s"; _ = c.WriteJSON(event) }
		emit(protocol.Event{Type: protocol.SessionReady, Provider: "nova"})
		for c.ReadJSON(&e) == nil {
			if e.Type == protocol.TurnCommit {
				break
			}
		}
		answer := func(id string) {
			emit(protocol.Event{Type: protocol.TurnStarted, TurnID: id})
			emit(protocol.Event{Type: protocol.AudioOutput, TurnID: id, Sequence: 1, SampleRate: 24000, Audio: "AAAAAA=="})
			emit(protocol.Event{Type: protocol.TurnCompleted, TurnID: id})
		}
		answer("first")
		for c.ReadJSON(&e) == nil {
			if e.Type == protocol.AudioAppend && strings.HasPrefix(e.Audio, "AQAB") {
				answer("second")
				break
			}
		}
		for c.ReadJSON(&e) == nil {
			if e.Type == protocol.SessionStop {
				emit(protocol.Event{Type: protocol.SessionStopped})
				return
			}
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	initial := filepath.Join(dir, "input.wav")
	followup := filepath.Join(dir, "confirm.wav")
	output := filepath.Join(dir, "output.wav")
	for path, pcm := range map[string][]byte{initial: make([]byte, 1024), followup: {1, 0, 1, 0}} {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = audio.WritePCM16WAV(file, pcm, 16000)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := runWithOptions(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), initial, output, 0, "nova", "", clientOptions{FollowupWAV: followup}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{output, output + ".turn-1.wav"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
}
