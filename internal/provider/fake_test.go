package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"stsmodel.local/poc/internal/protocol"
)

func TestFakeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- (Fake{}).Respond(ctx, nil, func(e protocol.Event) error {
			if e.Type == protocol.AudioOutput {
				cancel()
			}
			return nil
		})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider leaked after cancellation")
	}
}
