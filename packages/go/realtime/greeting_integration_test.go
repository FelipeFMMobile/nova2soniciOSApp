package nova

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in: one short Nova greeting, without microphone input or user credentials.
func TestLiveNovaGreeting(t *testing.T) {
	if os.Getenv("STS_NOVA_GREETING_TEST") != "1" {
		t.Skip("requires opt-in LiteLLM Nova endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	stream, err := Open(ctx, os.Getenv("STS_LITELLM_URL"), os.Getenv("STS_LITELLM_API_KEY"), "nova-sonic", "greeting-verification", "greeting-verification", "Fale português brasileiro. Ao ser solicitado, diga apenas a apresentação pedida e aguarde a resposta. Não chame ferramentas.", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := stream.SendText("Diga apenas: Sou agente STS para logar sua conta, deseja continuar?"); err != nil {
		t.Fatal(err)
	}
	sawAudio := false
	text := ""
	mapper := NewMapper(nil)
	mapper.StartAssistant("opening")
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("greeting timed out; audio=%v, transcript bytes=%d", sawAudio, len(text))
		case event, ok := <-stream.Events():
			if !ok {
				t.Fatal("greeting stream closed")
			}
			events, err := mapper.Map(event)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range events {
				if e.Type == "audio.output" {
					sawAudio = true
				}
				if e.Type == "transcript" && e.Role == "ASSISTANT" && e.Stage == "FINAL" {
					text += e.Text
				}
				if e.Type == "turn.completed" {
					lower := strings.ToLower(text)
					if !sawAudio || !strings.Contains(lower, "conta") || !strings.Contains(lower, "continuar") {
						t.Fatalf("invalid greeting; audio=%v, transcript bytes=%d", sawAudio, len(text))
					}
					t.Log("Nova Sonic returned greeting audio and its final transcript without microphone input")
					return
				}
			}
		}
	}
}
