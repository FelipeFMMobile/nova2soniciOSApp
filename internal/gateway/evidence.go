package gateway

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/orchestrator"
)

// Opt-in private evidence is distinct from normal ephemeral structured logs.
func openEvidence(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("evidence path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func (t *toolRuntime) evidenceResult(j orchestrator.Job, r mcp.Result) error {
	if t.evidence == nil {
		return nil
	}
	server, tool := "legacy", j.Name
	if parts := strings.SplitN(j.Name, ".", 2); len(parts) == 2 && len(strings.Split(j.Name, ".")) > 2 {
		server, tool = parts[0], parts[1]
	}
	var arguments any = j.Args
	if !json.Valid(j.Args) {
		arguments = string(j.Args)
	}
	return json.NewEncoder(t.evidence).Encode(map[string]any{"session_id": t.sessionID, "operation_id": j.ID, "server": server, "tool": tool, "arguments": arguments, "result": r})
}
