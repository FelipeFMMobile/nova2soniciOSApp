package gateway

import (
	"context"
	"stsmodel.local/agentwire"
	"testing"
)

type parityFactory func(context.Context, agentwire.AgentSessionStart) (agentwire.AgentSession, error)

func (f parityFactory) Open(ctx context.Context, start agentwire.AgentSessionStart) (agentwire.AgentSession, error) {
	return f(ctx, start)
}

// Execute each scenario independently: never duplicate a live session or mutation.
// The remote branch runs the actual assistant runtime through the authenticated
// WebSocket; only the upstream model is simulated. SQLite MCPs remain real.
func TestAgentRolloutParity(t *testing.T) {
	scenarios := []struct {
		name string
		run  func(*testing.T)
	}{
		{"continuous-audio-barge-in", TestNovaForwardsBeforeCommitAndUsesNativeBargeIn},
		{"renewal-history", TestNovaRenewalReplaysFinalHistory},
		{"cancel-provider-failure", TestNovaCancelAndProviderFailure},
		{"tool-discovery", TestNovaDiscoversAndExecutesMCP},
		{"later-confirmation", TestNovaDeletionRequiresLaterFinalUserConfirmation},
		{"tool-timeout", TestNovaToolTimeoutReturnsFailure},
		{"durable-notes-replay", TestGatewayRealStdioSQLiteRetryAcrossSessions},
		{"microphone-during-tool", TestMicrophoneContinuesWhileMCPRuns},
		{"disconnect-after-mutation", TestMutationCommittedBeforeDisconnectReplaysByRequestID},
		{"private-logs", TestOperationalLogsExcludeConversationContent},
		{"multiple-mcps", TestTwoRealMCPsMockNovaScenarios},
		{"renewal-routing", TestMultiNovaRenewalKeepsRouting},
		{"timeout-isolation", TestMultiMCPAudioAndTimeoutIsolation},
		{"durable-agenda-replay", TestAgendaGatewayDurableRetryAcrossSessions},
	}
	for _, mode := range []string{"inprocess", "remote", "inprocess"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("STS_TEST_AGENT_MODE", mode)
			for _, scenario := range scenarios {
				t.Run(scenario.name, scenario.run)
			}
		})
	}
}
