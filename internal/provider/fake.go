package provider

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math"
	"time"

	"stsmodel.local/poc/internal/protocol"
)

// Fake emits deterministic text and a tone, NOT synthesized speech or real ASR.
type Fake struct{}

func (Fake) ID() string { return "fake" }

func (Fake) Respond(ctx context.Context, _ []byte, emit func(protocol.Event) error) error {
	for _, event := range []protocol.Event{
		{Type: protocol.Transcript, Role: "USER", Text: "Áudio recebido (simulação, sem transcrição real)."},
		{Type: protocol.Transcript, Role: "ASSISTANT", Text: "Olá! Esta é uma resposta simulada. O gateway está funcionando."},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	for chunk := 0; chunk < 20; chunk++ {
		timer := time.NewTimer(40 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		pcm := make([]byte, 960*2)
		for sample := 0; sample < 960; sample++ {
			position := chunk*960 + sample
			value := int16(1600 * math.Sin(2*math.Pi*440*float64(position)/24000))
			binary.LittleEndian.PutUint16(pcm[sample*2:], uint16(value))
		}
		if err := emit(protocol.Event{Type: protocol.AudioOutput, Audio: base64.StdEncoding.EncodeToString(pcm), SampleRate: 24000}); err != nil {
			return err
		}
	}
	return ctx.Err()
}
