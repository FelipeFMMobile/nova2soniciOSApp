package nova

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/session"
)

type content struct {
	turn, kind, role, stage, text string
	rate                          int
}
type Mapper struct {
	TurnID      string
	State       string
	History     []HistoryMessage
	nextInput   string
	answered    map[string]bool
	cancelled   map[string]bool
	contents    map[string]*content
	users       map[string]string
	sequence    uint64
	started     time.Time
	firstAudio  time.Duration
	waitingUser bool
}

func NewMapper(history []HistoryMessage) *Mapper {
	return &Mapper{State: session.Idle, History: history, answered: map[string]bool{}, cancelled: map[string]bool{}, contents: map[string]*content{}, users: map[string]string{}}
}

func (m *Mapper) StartInput(id string) []protocol.Event {
	var events []protocol.Event
	if m.State == session.Responding {
		events = m.Interrupt()
	}
	m.nextInput = id
	m.TurnID = id
	m.State = session.Listening
	m.started = time.Now()
	m.firstAudio = 0
	m.sequence = 0
	m.waitingUser = true
	return append(events, protocol.Event{Type: protocol.TurnStarted, TurnID: id}, m.state())
}
func (m *Mapper) Commit()                 { m.started = time.Now() }
func (m *Mapper) FirstAudioMicros() int64 { return m.firstAudio.Microseconds() }
func (m *Mapper) state() protocol.Event {
	return protocol.Event{Type: protocol.SessionState, TurnID: m.TurnID, State: m.State}
}
func (m *Mapper) Interrupt() []protocol.Event {
	if m.TurnID == "" || m.cancelled[m.TurnID] || m.State == session.Idle {
		return nil
	}
	at := time.Now()
	m.cancelled[m.TurnID] = true
	m.State = session.Idle
	return []protocol.Event{{Type: protocol.TurnInterrupted, TurnID: m.TurnID, Metrics: &protocol.Metrics{InterruptionMS: float64(time.Since(at).Microseconds()) / 1000}}, m.state()}
}

func (m *Mapper) Map(e Event) ([]protocol.Event, error) {
	if e.Err != nil {
		return nil, e.Err
	}
	if e.Kind == "interrupted" {
		return m.Interrupt(), nil
	}
	if e.Kind == "completionEnd" && e.StopReason == "END_TURN" && m.State == session.Responding && m.sequence > 0 {
		return m.complete(), nil
	}
	if e.Kind == "contentStart" {
		if e.Role == "USER" {
			m.waitingUser = false
			var events []protocol.Event
			if m.State == session.Responding {
				events = m.Interrupt()
			}
			if m.TurnID == "" || m.answered[m.TurnID] || m.cancelled[m.TurnID] {
				id := m.nextInput
				if id == "" || m.answered[id] || m.cancelled[id] {
					id = rand.Text()
				}
				m.TurnID = id
				m.started = time.Now()
				m.firstAudio = 0
				m.sequence = 0
				events = append(events, protocol.Event{Type: protocol.TurnStarted, TurnID: id})
			}
			m.State = session.Listening
			if len(m.answered)+len(m.cancelled) >= session.MaxTurns {
				return nil, fmt.Errorf("Nova session turn limit reached")
			}
			m.contents[e.ContentID] = &content{turn: m.TurnID, kind: e.Type, role: e.Role, stage: e.Stage}
			return append(events, m.state()), nil
		}
		if m.waitingUser {
			return nil, nil
		}
		if len(m.contents) >= 1024 {
			return nil, fmt.Errorf("Nova content limit reached")
		}
		m.contents[e.ContentID] = &content{turn: m.TurnID, kind: e.Type, role: e.Role, stage: e.Stage, rate: e.Rate}
		if e.Role == "ASSISTANT" && (e.Type == "AUDIO" || e.Stage == "SPECULATIVE") && !m.cancelled[m.TurnID] {
			m.State = session.Responding
			return []protocol.Event{m.state()}, nil
		}
		return nil, nil
	}
	c := m.contents[e.ContentID]
	if c == nil || m.cancelled[c.turn] || c.turn != m.TurnID {
		return nil, nil
	}
	switch e.Kind {
	case "textOutput":
		if len(c.text)+len(e.Text) > 8192 {
			return nil, fmt.Errorf("Nova transcript limit reached")
		}
		c.text += e.Text
		return []protocol.Event{{Type: protocol.Transcript, TurnID: c.turn, Role: c.role, Stage: c.stage, Text: e.Text}}, nil
	case "audioOutput":
		if m.State != session.Responding {
			return nil, nil
		}
		pcm, err := base64.StdEncoding.Strict().DecodeString(e.Audio)
		if err != nil || len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > 256*1024 || c.rate != 24000 {
			return nil, fmt.Errorf("invalid Nova PCM format")
		}
		m.sequence++
		if m.firstAudio == 0 {
			m.firstAudio = time.Since(m.started)
		}
		return []protocol.Event{{Type: protocol.AudioOutput, TurnID: c.turn, Sequence: m.sequence, SampleRate: c.rate, Audio: e.Audio}}, nil
	case "toolUse":
		return []protocol.Event{{Type: protocol.ToolStarted, TurnID: c.turn, Tool: &protocol.Tool{OperationID: e.ToolID, Name: e.ToolName, Arguments: e.ToolArguments}}}, nil
	case "contentEnd":
		delete(m.contents, e.ContentID)
		if e.StopReason == "INTERRUPTED" {
			return m.Interrupt(), nil
		}
		if c.kind == "TEXT" && c.stage == "FINAL" {
			if c.role == "USER" {
				if previous := m.users[c.turn]; previous != "" {
					c.text = previous + " " + c.text
				}
				if len(c.text) > 8192 {
					return nil, fmt.Errorf("Nova user transcript limit reached")
				}
				m.users[c.turn] = c.text
			} else if c.role == "ASSISTANT" && m.users[c.turn] != "" {
				m.History = append(m.History, HistoryMessage{Role: "USER", Content: m.users[c.turn]}, HistoryMessage{Role: "ASSISTANT", Content: c.text})
				for len(m.History) > 32 || historyBytes(m.History) > 65536 {
					m.History = m.History[2:]
				}
				delete(m.users, c.turn)
			}
			if c.role == "ASSISTANT" && e.StopReason == "END_TURN" && m.State == session.Responding && m.sequence > 0 {
				return m.complete(), nil
			}
		}
		if c.kind == "AUDIO" && e.StopReason == "END_TURN" && m.State == session.Responding {
			return m.complete(), nil
		}
	}
	return nil, nil
}

func (m *Mapper) complete() []protocol.Event {
	m.State = session.Idle
	m.answered[m.TurnID] = true
	return []protocol.Event{{Type: protocol.TurnCompleted, TurnID: m.TurnID, Metrics: &protocol.Metrics{FirstAudioMS: float64(m.firstAudio.Microseconds()) / 1000, DurationMS: float64(time.Since(m.started).Microseconds()) / 1000}}, m.state()}
}

func historyBytes(messages []HistoryMessage) int {
	size := 0
	for _, message := range messages {
		size += len(message.Content)
	}
	return size
}
