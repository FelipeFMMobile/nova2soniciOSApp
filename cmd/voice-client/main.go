// voice-client demonstrates the gateway contract without microphone or AWS.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"stsmodel.local/poc/internal/audio"
	"stsmodel.local/poc/internal/protocol"
	"stsmodel.local/poc/internal/session"
)

type result struct {
	event protocol.Event
	err   error
}

func main() {
	url := flag.String("url", "ws://127.0.0.1:8080/v1/voice", "gateway WebSocket URL")
	wav := flag.String("wav", "", "optional mono PCM16 16 kHz input WAV (maximum 30 seconds)")
	output := flag.String("output", "/private/tmp/sts-fake-response.wav", "output WAV path")
	cancelAfter := flag.Duration("cancel-after", 0, "cancel the simulated response after this duration")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *url, *wav, *output, *cancelAfter); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, url, wavPath, outputPath string, cancelAfter time.Duration) error {
	if cancelAfter < 0 {
		return fmt.Errorf("cancel-after must not be negative")
	}
	pcm := make([]byte, 16000) // Half a second of silence: fake does not perform ASR.
	if wavPath != "" {
		file, err := os.Open(wavPath)
		if err != nil {
			return err
		}
		pcm, err = audio.ReadPCM16WAV(file, 16000, session.MaxTurnBytes)
		file.Close()
		if err != nil {
			return err
		}
	}
	headers := http.Header{}
	if token := os.Getenv("STS_DEVELOPMENT_TOKEN"); token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	c, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer c.Close()
	c.SetReadLimit(512 * 1024)
	write := func(e protocol.Event) error {
		e.Version = 1
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return c.WriteJSON(e)
	}
	if err := write(protocol.Event{Type: protocol.SessionStart, Provider: "fake"}); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ready protocol.Event
	if err := c.ReadJSON(&ready); err != nil {
		return err
	}
	if ready.Type != protocol.SessionReady || ready.Version != 1 {
		return fmt.Errorf("session not ready: %s %s", ready.Code, ready.Message)
	}
	sid, tid := ready.SessionID, rand.Text()
	fmt.Printf("session=%s provider=%s turn=%s\n", sid, ready.Provider, tid)
	sequence := uint64(0)
	for offset := 0; offset < len(pcm); offset += 1024 {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(offset+1024, len(pcm))
		sequence++
		if err := write(protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: tid, Sequence: sequence, SampleRate: 16000, Audio: base64.StdEncoding.EncodeToString(pcm[offset:end])}); err != nil {
			return err
		}
	}
	if err := write(protocol.Event{Type: protocol.TurnCommit, SessionID: sid, TurnID: tid}); err != nil {
		return err
	}
	in := make(chan result, 8)
	done := make(chan struct{})
	readCtx, readCancel := context.WithCancel(ctx)
	go func() {
		defer close(done)
		for {
			_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
			var e protocol.Event
			err := c.ReadJSON(&e)
			select {
			case in <- result{e, err}:
			case <-readCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() { readCancel(); c.Close(); <-done }()
	var cancellation <-chan time.Time
	if cancelAfter > 0 {
		timer := time.NewTimer(cancelAfter)
		defer timer.Stop()
		cancellation = timer.C
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	var response []byte
	var audioSequence uint64
	sampleRate := 0
	stopping := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("voice demo timed out")
		case <-cancellation:
			cancellation = nil
			if !stopping {
				if err := write(protocol.Event{Type: protocol.TurnCancel, SessionID: sid, TurnID: tid}); err != nil {
					return err
				}
			}
		case r := <-in:
			if r.err != nil {
				return r.err
			}
			e := r.event
			if e.Version != 1 || e.SessionID != sid {
				return fmt.Errorf("protocol/session mismatch")
			}
			switch e.Type {
			case protocol.Error:
				return fmt.Errorf("%s: %s", e.Code, e.Message)
			case protocol.Transcript:
				fmt.Printf("%s: %s\n", e.Role, e.Text)
			case protocol.SessionState:
				fmt.Printf("state=%s\n", e.State)
			case protocol.AudioOutput:
				if stopping || e.TurnID != tid || e.Sequence != audioSequence+1 {
					return fmt.Errorf("stale or out-of-order audio")
				}
				if sampleRate != 0 && sampleRate != e.SampleRate {
					return fmt.Errorf("output sample rate changed within turn")
				}
				decoded, err := base64.StdEncoding.Strict().DecodeString(e.Audio)
				if err != nil || len(decoded)%2 != 0 {
					return fmt.Errorf("invalid output PCM")
				}
				if len(response)+len(decoded) > 30*48000*2 {
					return fmt.Errorf("output exceeds demo buffer limit")
				}
				sampleRate = e.SampleRate
				audioSequence++
				response = append(response, decoded...)
			case protocol.TurnCompleted:
				file, err := os.Create(outputPath)
				if err != nil {
					return err
				}
				err = audio.WritePCM16WAV(file, response, sampleRate)
				closeErr := file.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				metrics, _ := json.Marshal(e.Metrics)
				fmt.Printf("audio=%s bytes=%d metrics=%s\n", outputPath, len(response), metrics)
				stopping = true
				cancellation = nil
				if err := write(protocol.Event{Type: protocol.SessionStop, SessionID: sid}); err != nil {
					return err
				}
			case protocol.TurnInterrupted:
				response = nil
				stopping = true
				fmt.Println("turn interrupted; output discarded")
				if err := write(protocol.Event{Type: protocol.SessionStop, SessionID: sid}); err != nil {
					return err
				}
			case protocol.SessionStopped:
				fmt.Println("session stopped")
				return nil
			}
		}
	}
}
