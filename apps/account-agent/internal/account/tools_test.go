package account

import (
	"encoding/json"
	"strings"
	"testing"

	orchestrator "stsmodel.local/actionruntime"
	mcp "stsmodel.local/mcpwire"
)

func TestAccountResultsExposeOnlyOutcomeToApple(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result mcp.Result
		status string
	}{
		{"profile.update_me", mcp.TextResult(map[string]string{"city": "private-canary"}, false), "completed"},
		{"profile.update_me", mcp.Failure("private-canary"), "completed"},
		{"identity.logout", cancelledResult(), "cancelled"},
		{"identity.logout", mcp.TextResult(map[string]string{"status": "signed_out"}, false), "signed_out"},
	} {
		event := toolResultEvent(orchestrator.Job{ID: "operation", Name: tc.name}, tc.result)
		converted, err := agentEventFromVoice(event)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(converted)
		if strings.Contains(string(raw), "private-canary") || !strings.Contains(string(raw), tc.status) {
			t.Fatal("Apple result leaked payload or reported the wrong outcome")
		}
		if event.Tool.Arguments != nil {
			t.Fatal("result repeated private arguments")
		}
	}
}

func TestUIConfirmationWithoutPendingOperationIsRejected(t *testing.T) {
	var runtime *toolRuntime
	if _, _, accepted := runtime.confirm("operation", true); accepted {
		t.Fatal("missing runtime accepted confirmation")
	}
}
