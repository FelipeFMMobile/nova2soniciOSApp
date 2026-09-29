package main

import (
	"log/slog"
	"os"

	agentconfig "stsmodel.local/personal-assistant-agent/config"
)

// The remote Agent Session listener is introduced in phase 4. This binary
// already has an independent build and validates the app-owned prompt so a
// broken artifact fails closed instead of starting with implicit instructions.
func main() {
	prompt, err := agentconfig.LoadPrompt(os.Getenv("STS_AGENT_PROMPT_FILE"))
	if err != nil {
		slog.Error("invalid personal assistant configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("personal assistant artifact ready", "prompt_bytes", len(prompt), "transport", "not-configured")
}
