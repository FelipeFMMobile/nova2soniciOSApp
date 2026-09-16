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
	output := flag.String("output", "", "output WAV path (default /private/tmp/sts-PROVIDER-response.wav)")
	selected := flag.String("provider", "fake", "fake or nova; nova requires an input WAV")
	bargeWAV := flag.String("barge-wav", "", "Nova-only: send a second WAV during the first response to test native barge-in")
	cancelAfter := flag.Duration("cancel-after", 0, "cancel the simulated response after this duration")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *output == "" {
		*output = "/private/tmp/sts-" + *selected + "-response.wav"
	}
	if err := runProvider(ctx, *url, *wav, *output, *cancelAfter, *selected, *bargeWAV); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, url, wavPath, outputPath string, cancelAfter time.Duration) error {
	return runProvider(ctx, url, wavPath, outputPath, cancelAfter, "fake")
}

func runProvider(ctx context.Context, url, wavPath, outputPath string, cancelAfter time.Duration, selected string, bargePaths ...string) error {
	if selected != "fake" && selected != "nova" {
		return fmt.Errorf("provider must be fake or nova")
	}
	if selected == "nova" && wavPath == "" {
		return fmt.Errorf("Nova requires -wav with PT-BR mono PCM16 at 16 kHz")
	}
	var bargeAudio []byte
	if len(bargePaths) > 0 && bargePaths[0] != "" {
		if selected != "nova" || cancelAfter != 0 {
			return fmt.Errorf("barge-wav requires Nova and cannot be combined with cancel-after")
		}
		file, err := os.Open(bargePaths[0])
		if err != nil {
			return err
		}
		bargeAudio, err = audio.ReadPCM16WAV(file, 16000, session.MaxTurnBytes)
		file.Close()
		if err != nil {
			return err
		}
	}
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
	if err := write(protocol.Event{Type: protocol.SessionStart, Provider: selected}); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
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
		if selected == "nova" {
			timer := time.NewTimer(32 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
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
	var nativeCancellation *time.Timer
	defer func() {
		if nativeCancellation != nil {
			nativeCancellation.Stop()
		}
	}()
	if cancelAfter > 0 && selected == "fake" {
		timer := time.NewTimer(cancelAfter)
		defer timer.Stop()
		cancellation = timer.C
	}
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	var response []byte
	var audioSequence uint64
	sampleRate := 0
	stopping := false
	outputTurnID := tid
	bargeStarted, bargeInterrupted := false, false
	bargeOffset := 0
	var silence <-chan time.Time
	if selected == "nova" {
		ticker := time.NewTicker(32 * time.Millisecond)
		defer ticker.Stop()
		silence = ticker.C
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("voice demo timed out")
		case <-silence:
			if !stopping {
				sequence++
				frame := make([]byte, 1024)
				if bargeStarted && bargeOffset < len(bargeAudio) {
					end := min(bargeOffset+1024, len(bargeAudio))
					copy(frame, bargeAudio[bargeOffset:end])
					bargeOffset = end
				}
				if err := write(protocol.Event{Type: protocol.AudioAppend, SessionID: sid, TurnID: tid, Sequence: sequence, SampleRate: 16000, Audio: base64.StdEncoding.EncodeToString(frame)}); err != nil {
					return err
				}
			}
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
			case protocol.TurnStarted:
				if selected == "nova" && e.TurnID != outputTurnID {
					outputTurnID = e.TurnID
					response = nil
					sampleRate = 0
					audioSequence = 0
				}
			case protocol.Error:
				return fmt.Errorf("%s: %s", e.Code, e.Message)
			case protocol.Transcript:
				fmt.Printf("%s: %s\n", e.Role, e.Text)
			case protocol.SessionState:
				fmt.Printf("state=%s\n", e.State)
			case protocol.AudioOutput:
				if len(bargeAudio) > 0 && !bargeStarted {
					bargeStarted = true
					fmt.Println("sending second speech through the same live microphone stream")
				}
				if selected == "nova" && cancelAfter > 0 && nativeCancellation == nil {
					nativeCancellation = time.NewTimer(cancelAfter)
					cancellation = nativeCancellation.C
				}
				if stopping || e.TurnID != outputTurnID || e.Sequence != audioSequence+1 {
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
				if len(bargeAudio) > 0 && !bargeInterrupted {
					return fmt.Errorf("Nova completed before native barge-in; interruption scenario did not pass")
				}
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
				sampleRate = 0
				audioSequence = 0
				if len(bargeAudio) > 0 && bargeStarted {
					bargeInterrupted = true
					fmt.Println("native barge-in confirmed; old audio discarded")
					continue
				}
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
