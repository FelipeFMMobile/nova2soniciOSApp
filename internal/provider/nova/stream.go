// Package nova adapts the private Python/Bedrock transport, not AWS credentials.
package nova

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Event struct {
	Kind          string
	ContentID     string
	Type          string
	Role          string
	Stage         string
	Text          string
	Audio         string
	Rate          int
	StopReason    string
	ToolID        string
	ToolName      string
	ToolArguments json.RawMessage
	Err           error
}

type Stream struct {
	conn   *websocket.Conn
	events chan Event
	done   chan struct{}
	once   sync.Once
	stop   func() bool
	cancel context.CancelFunc
}

func Open(ctx context.Context, url string, history []HistoryMessage, toolSets ...[]map[string]any) (*Stream, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Nova bridge unavailable")
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	fail := func() { stop(); _ = conn.Close() }
	conn.SetReadLimit(1024 * 1024)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	start := map[string]any{"type": "session.start", "history": history}
	if len(toolSets) > 0 {
		start["tools"] = toolSets[0]
	}
	if err := conn.WriteJSON(start); err != nil {
		fail()
		return nil, errors.New("Nova bridge startup failed")
	}
	_ = conn.SetReadDeadline(time.Now().Add(25 * time.Second))
	for {
		var message bridgeMessage
		if err := conn.ReadJSON(&message); err != nil {
			fail()
			return nil, errors.New("Nova bridge handshake failed")
		}
		if message.Type == "session.ready" {
			break
		}
		if message.Type != "nova.event" {
			fail()
			return nil, errors.New("Nova bridge rejected session")
		}
		if _, ok := message.Payload.Event["bridgeError"]; ok {
			fail()
			return nil, errors.New("Nova inference startup failed")
		}
	}
	readCtx, cancel := context.WithCancel(ctx)
	s := &Stream{conn: conn, events: make(chan Event, 32), done: make(chan struct{}), stop: stop, cancel: cancel}
	go s.read(readCtx)
	return s, nil
}

func (s *Stream) Events() <-chan Event { return s.events }
func (s *Stream) SendAudio(encoded string) error {
	_ = s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := s.conn.WriteJSON(map[string]string{"type": "audio.append", "audio": encoded}); err != nil {
		return errors.New("Nova bridge write failed")
	}
	return nil
}

// SendToolResult is serialized with audio by the gateway session owner.
func (s *Stream) SendToolResult(id string, result any) error {
	data, err := json.Marshal(result)
	if err != nil || len(data) > 65536 {
		return errors.New("invalid tool result")
	}
	_ = s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err = s.conn.WriteJSON(map[string]string{"type": "tool.result", "toolUseId": id, "content": string(data)}); err != nil {
		return errors.New("Nova tool result transport failed")
	}
	return nil
}

func (s *Stream) Close() {
	s.once.Do(func() {
		s.stop()
		_ = s.conn.SetWriteDeadline(time.Now().Add(time.Second))
		_ = s.conn.WriteJSON(map[string]string{"type": "session.stop"})
		s.cancel()
		_ = s.conn.Close()
		<-s.done
	})
}

type bridgeMessage struct {
	Type    string `json:"type"`
	Payload struct {
		Event map[string]json.RawMessage `json:"event"`
	} `json:"payload"`
}

func (s *Stream) read(ctx context.Context) {
	defer close(s.done)
	defer close(s.events)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		var message bridgeMessage
		if err := s.conn.ReadJSON(&message); err != nil {
			return
		}
		if message.Type != "nova.event" {
			select {
			case s.events <- Event{Err: errors.New("Nova inference failed")}:
			case <-ctx.Done():
			}
			return
		}
		for kind, raw := range message.Payload.Event {
			event, err := decode(kind, raw)
			if err != nil {
				event = Event{Err: errors.New("invalid Nova provider event")}
			}
			select {
			case s.events <- event:
			case <-ctx.Done():
				return
			}
			if event.Err != nil {
				return
			}
		}
	}
}

func decode(kind string, raw json.RawMessage) (Event, error) {
	var fields struct {
		ContentID   string `json:"contentId"`
		Type        string `json:"type"`
		Role        string `json:"role"`
		Content     string `json:"content"`
		Additional  string `json:"additionalModelFields"`
		StopReason  string `json:"stopReason"`
		ToolID      string `json:"toolUseId"`
		ToolName    string `json:"toolName"`
		AudioConfig struct {
			Rate int `json:"sampleRateHertz"`
		} `json:"audioOutputConfiguration"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Event{}, err
	}
	if kind == "bridgeError" {
		return Event{Err: errors.New("Nova inference failed")}, nil
	}
	e := Event{Kind: kind, ContentID: fields.ContentID, Type: fields.Type, Role: fields.Role, Text: fields.Content, Rate: fields.AudioConfig.Rate, StopReason: fields.StopReason, ToolID: fields.ToolID, ToolName: fields.ToolName}
	if fields.Additional != "" {
		var additional struct {
			Stage string `json:"generationStage"`
		}
		if err := json.Unmarshal([]byte(fields.Additional), &additional); err != nil {
			return Event{}, err
		}
		e.Stage = additional.Stage
	}
	if kind == "audioOutput" {
		e.Audio = fields.Content
		e.Text = ""
	}
	if kind == "toolUse" {
		e.ToolArguments = json.RawMessage(fields.Content)
		if !json.Valid(e.ToolArguments) {
			return Event{}, errors.New("invalid tool arguments")
		}
	}
	if kind == "textOutput" {
		var signal struct {
			Interrupted bool `json:"interrupted"`
		}
		if json.Unmarshal([]byte(fields.Content), &signal) == nil && signal.Interrupted {
			e.Kind = "interrupted"
		}
	}
	return e, nil
}
