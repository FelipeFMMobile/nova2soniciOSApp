// Package session owns per-connection state. Its owner must serialize calls.
package session

import (
	"crypto/rand"
	"fmt"

	"stsmodel.local/poc/internal/protocol"
)

const (
	Idle         = "idle"
	Listening    = "listening"
	Responding   = "responding"
	Closed       = "closed"
	MaxTurnBytes = 30 * protocol.InputSampleRate * 2
	MaxTurns     = 128
)

type Session struct {
	ID        string
	State     string
	TurnID    string
	sequence  uint64
	pcm       []byte
	usedTurns map[string]bool
}

func New() *Session {
	return &Session{ID: rand.Text(), State: Idle, usedTurns: make(map[string]bool)}
}

// Append returns the old turn ID when a valid new turn interrupts a response.
// Validation precedes mutation: a malformed frame cannot cancel valid work.
func (s *Session) Append(event protocol.Event) (string, error) {
	if s.State == Closed || event.SessionID != s.ID {
		return "", fmt.Errorf("invalid session")
	}
	pcm, err := protocol.DecodeAudio(event.Audio)
	if err != nil {
		return "", err
	}
	newTurn := s.State == Idle || (s.State == Responding && event.TurnID != s.TurnID)
	if newTurn {
		if event.Sequence != 1 || s.usedTurns[event.TurnID] || len(s.usedTurns) >= MaxTurns {
			return "", fmt.Errorf("new turn requires unused turnId and sequence 1; maximum 128 turns")
		}
	} else if s.State != Listening || event.TurnID != s.TurnID || event.Sequence != s.sequence+1 {
		return "", fmt.Errorf("unexpected turn or audio sequence")
	}
	if !newTurn && len(s.pcm)+len(pcm) > MaxTurnBytes {
		return "", fmt.Errorf("turn audio exceeds 30 seconds")
	}
	interrupted := ""
	if newTurn {
		if s.State == Responding {
			interrupted = s.TurnID
		}
		s.TurnID = event.TurnID
		s.usedTurns[event.TurnID] = true
		s.pcm = nil
		s.State = Listening
	}
	s.sequence = event.Sequence
	s.pcm = append(s.pcm, pcm...)
	return interrupted, nil
}

func (s *Session) Commit(turnID string) ([]byte, error) {
	if s.State != Listening || turnID != s.TurnID || len(s.pcm) == 0 {
		return nil, fmt.Errorf("no listening turn to commit")
	}
	pcm := s.pcm
	s.pcm = nil
	s.State = Responding
	return pcm, nil
}

func (s *Session) Cancel(turnID string) error {
	if turnID != s.TurnID || (s.State != Listening && s.State != Responding) {
		return fmt.Errorf("no active matching turn")
	}
	s.State = Idle
	s.pcm = nil
	return nil
}

func (s *Session) Complete(turnID string) bool {
	if s.State != Responding || s.TurnID != turnID {
		return false
	}
	s.State = Idle
	return true
}

func (s *Session) Close() { s.State = Closed; s.pcm = nil }
