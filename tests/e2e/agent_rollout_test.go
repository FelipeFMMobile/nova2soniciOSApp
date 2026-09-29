package e2e

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Exercise production binaries and unchanged clients. The model is a local
// deterministic Realtime peer; this test never calls AWS or reads credentials.
func TestAgentRolloutProcesses(t *testing.T) {
	root := repositoryRoot(t)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
		_ = c.WriteJSON(map[string]any{"type": "session.created", "session": map[string]any{}})
		var message map[string]any
		if c.ReadJSON(&message) != nil {
			return
		}
		_ = c.WriteJSON(map[string]any{"type": "session.updated", "session": map[string]any{}})
		responded := false
		for c.ReadJSON(&message) == nil {
			if message["type"] != "input_audio_buffer.append" || responded {
				continue
			}
			responded = true
			for _, event := range []map[string]any{
				{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user", "transcript": "rollout-private-7419"},
				{"type": "response.content_part.added", "item_id": "answer", "content_index": 0, "part": map[string]any{"type": "audio"}},
				{"type": "response.audio.delta", "item_id": "answer", "content_index": 0, "delta": "AAAAAA=="},
				{"type": "response.audio.done", "item_id": "answer", "content_index": 0},
				{"type": "response.done", "response": map[string]any{"status": "completed"}},
			} {
				if c.WriteJSON(event) != nil {
					return
				}
			}
		}
	}))
	defer model.Close()
	base := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !strings.HasPrefix(key, "STS_") && !strings.HasPrefix(key, "AWS_") && !strings.HasPrefix(key, "NOVA_") {
			base = append(base, item)
		}
	}
	runtimeEnv := []string{"STS_PROVIDER=nova", "STS_AGENT_PROMPT_FILE=" + filepath.Join(root, "apps/personal-assistant-agent/prompts/system.md"), "STS_LITELLM_URL=" + model.URL, "STS_LITELLM_API_KEY=test-model-key", "STS_MCP_BACKEND=stdio"}
	for _, mode := range []string{"inprocess", "remote", "inprocess"} {
		t.Run(mode, func(t *testing.T) {
			gatewayAddress := rolloutAddress(t)
			gatewayEnv := append(append([]string{}, base...), "STS_ENV=development", "STS_PROVIDER=nova", "STS_AGENT_MODE="+mode, "STS_DEVELOPMENT_TOKEN=apple-test-token", "STS_GATEWAY_ADDRESS="+gatewayAddress)
			if mode == "remote" {
				agentAddress := rolloutAddress(t)
				agentEnv := append(append([]string{}, base...), runtimeEnv...)
				agentEnv = append(agentEnv, "STS_AGENT_ADDRESS="+agentAddress, "STS_AGENT_TOKEN=rollout-internal-token")
				launchRollout(t, root, "personal-assistant-agent", agentEnv, agentAddress)
				catalog, _ := json.Marshal([]map[string]string{{"id": "personal-assistant", "endpoint": "ws://" + agentAddress + "/v1/session", "protocol": "agent-session/v1", "token_env": "STS_AGENT_TOKEN"}})
				gatewayEnv = append(gatewayEnv, "STS_AGENT_TOKEN=rollout-internal-token", "STS_AGENT_CATALOG="+string(catalog))
			} else {
				gatewayEnv = append(gatewayEnv, runtimeEnv...)
			}
			launchRollout(t, root, "gateway", gatewayEnv, gatewayAddress)
			dir := t.TempDir()
			input := filepath.Join(dir, "input.wav")
			output := filepath.Join(dir, "output.wav")
			// PCM16 mono WAV: synthetic silence only.
			wav := make([]byte, 44+640)
			copy(wav, "RIFF")
			binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
			copy(wav[8:], "WAVEfmt ")
			binary.LittleEndian.PutUint32(wav[16:], 16)
			binary.LittleEndian.PutUint16(wav[20:], 1)
			binary.LittleEndian.PutUint16(wav[22:], 1)
			binary.LittleEndian.PutUint32(wav[24:], 16000)
			binary.LittleEndian.PutUint32(wav[28:], 32000)
			binary.LittleEndian.PutUint16(wav[32:], 2)
			binary.LittleEndian.PutUint16(wav[34:], 16)
			copy(wav[36:], "data")
			binary.LittleEndian.PutUint32(wav[40:], 640)
			if err := os.WriteFile(input, wav, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(root, "bin/voice-client"), "-url", "ws://"+gatewayAddress+"/v1/voice", "-provider", "nova", "-wav", input, "-output", output)
			cmd.Env = append(append([]string{}, base...), "STS_DEVELOPMENT_TOKEN=apple-test-token")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("CLI failed: %v %s", err, out)
			}
			data, err := os.ReadFile(output)
			if err != nil || len(data) != 48 {
				t.Fatalf("CLI output mismatch: bytes=%d err=%v", len(data), err)
			}
			if os.Getenv("STS_ROLLOUT_APPLE") == "1" {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				cmd := exec.CommandContext(ctx, "swift", "test", "--package-path", filepath.Join(root, "apps/apple/VoiceCore"), "--scratch-path", "/private/tmp/sts-rollout-apple", "--filter", "VoiceSocketTests/testFakeGatewayLifecycleAndReconnect")
				cmd.Env = append(append([]string{}, base...), "STS_APPLE_E2E_URL=ws://"+gatewayAddress+"/v1/voice", "STS_APPLE_E2E_PROVIDER=nova")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("Apple VoiceCore failed: %v %s", err, out)
				}
			}
		})
	}
}

func rolloutAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	_ = l.Close()
	return address
}
func launchRollout(t *testing.T, root, name string, env []string, address string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), name+".log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(root, "bin", name))
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = log.Close()
		data, _ := os.ReadFile(logPath)
		for _, sentinel := range []string{"rollout-private-7419", "rollout-internal-token", "test-model-key"} {
			if strings.Contains(string(data), sentinel) {
				t.Errorf("%s log leaked content or credential", name)
			}
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("%s did not start: %s", name, data)
}
