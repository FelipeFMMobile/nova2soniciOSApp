package protocol

import (
	"testing"
)

func TestDecodeClient(t *testing.T) {
	valid := []string{
		`{"version":1,"type":"session.start","provider":"fake"}`,
		`{"version":1,"type":"audio.append","sessionId":"s","turnId":"t","sequence":1,"sampleRate":16000,"audio":"AAAA"}`,
		`{"version":1,"type":"turn.commit","sessionId":"s","turnId":"t"}`,
		`{"version":1,"type":"turn.cancel","sessionId":"s","turnId":"t"}`,
		`{"version":1,"type":"session.stop","sessionId":"s"}`,
		`{"version":1,"type":"session.start","provider":"nova","requestId":"retry-1"}`,
		`{"version":1,"type":"tool.confirm","sessionId":"s","tool":{"operationId":"delete-1","approved":true}}`,
	}
	// AAAA decodes to three bytes and is intentionally not valid PCM16.
	valid[1] = `{"version":1,"type":"audio.append","sessionId":"s","turnId":"t","sequence":1,"sampleRate":16000,"audio":"AAAAAA=="}`
	for _, data := range valid {
		if _, err := DecodeClient([]byte(data)); err != nil {
			t.Errorf("%s: %v", data, err)
		}
	}
	invalid := []string{
		`{}`, `{"version":2,"type":"session.start"}`, `{"version":1,"type":"session.start","unknown":true}`,
		`{"version":1,"type":"session.start"} {}`, `{"version":1,"type":"audio.output"}`,
		`{"version":1,"type":"session.start","sessionId":"s"}`,
		`{"version":1,"type":"turn.commit","sessionId":"s"}`,
		`{"version":1,"type":"session.stop","sessionId":"s","text":"spoof"}`,
		`{"version":1,"type":"audio.append","sessionId":"s","turnId":"t","sequence":1,"sampleRate":24000,"audio":"AAAAAA=="}`,
		`{"version":1,"type":"tool.confirm","sessionId":"s","tool":{"operationId":"delete-1"}}`,
		`{"version":1,"type":"tool.confirm","sessionId":"s","tool":{"operationId":"delete-1","approved":true,"arguments":{"id":"x"}}}`,
		`{"version":1,"type":"session.stop","sessionId":"s","requestId":"retry-1"}`,
	}
	for _, data := range invalid {
		if _, err := DecodeClient([]byte(data)); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
}

func TestDecodeAudioRejectsMalformedPCM(t *testing.T) {
	for _, data := range []string{"", "not base64", "AAAA", "AA=="} {
		if _, err := DecodeAudio(data); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
}
