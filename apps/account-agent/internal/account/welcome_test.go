package account

import (
	"context"
	"stsmodel.local/agentwire"
	"testing"
	"time"
)

func TestWelcomeAnswerRequiresUnambiguousConsent(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"Sim!", 1}, {"sim, quero continuar", 1}, {"Pode continuar.", 1},
		{"Não.", -1}, {"não quero continuar", -1}, {"agora nao", -1},
		{"talvez", 0}, {"não sei se sim", 0}, {"sim mas não agora", 0}, {"meu nome é Simão", 0},
	} {
		if got := welcomeAnswer(tc.text); got != tc.want {
			t.Errorf("%q: %d != %d", tc.text, got, tc.want)
		}
	}
}

func TestFakeWaitsForConsentAndDeclineClosesWithoutMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s := newFake(ctx, agentwire.AgentSessionStart{SessionID: "welcome-test"}, nil)
	defer s.Close()
	for _, want := range []string{agentwire.AgentSessionReady, agentwire.OutputTranscript, agentwire.ToolConfirmation} {
		select {
		case event := <-s.Events():
			if event.Type != want {
				t.Fatalf("%s != %s", event.Type, want)
			}
		case <-ctx.Done():
			t.Fatal("missing welcome")
		}
	}
	// No MCP backend exists: opening it before consent would panic.
	select {
	case event := <-s.Events():
		t.Fatalf("unexpected pre-consent event: %s", event.Type)
	default:
	}
	if err := s.Send(ctx, agentwire.AgentCommand{Type: agentwire.InputConfirmation, SessionID: "welcome-test", OperationID: welcomeOperation, Approved: false}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-s.Events():
		if event.Type != agentwire.AgentSessionClosed {
			t.Fatal(event.Type)
		}
	case <-ctx.Done():
		t.Fatal("decline did not close session")
	}
}
