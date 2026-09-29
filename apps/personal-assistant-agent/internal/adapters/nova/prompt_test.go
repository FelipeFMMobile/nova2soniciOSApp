package nova

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentconfig "stsmodel.local/personal-assistant-agent/config"
)

func TestSystemPromptCharacterization(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "prompts", "system.md"))
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := agentconfig.LoadPrompt(path)
	if err != nil {
		t.Fatal(err)
	}
	data := strings.TrimSpace(string(mustReadFixture(t)))
	if prompt != data {
		t.Fatal("system prompt changed without updating the characterization baseline")
	}
}

func mustReadFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/system-prompt.txt")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
