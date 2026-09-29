package nova

import (
	"os"
	"strings"
	"testing"
)

func TestSystemPromptCharacterization(t *testing.T) {
	data, err := os.ReadFile("testdata/system-prompt.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSuffix(string(data), "\n"); got != systemPrompt {
		t.Fatal("system prompt changed without updating the characterization baseline")
	}
}
