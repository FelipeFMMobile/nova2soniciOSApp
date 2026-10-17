package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	orchestrator "stsmodel.local/actionruntime"
	"stsmodel.local/agentwire"
	"stsmodel.local/mcpwire"
	"testing"
	"time"
)

func TestLiveGuardrails(t *testing.T) {
	if os.Getenv("STS_ACCOUNT_INTEGRATION") != "1" {
		t.Skip("requires isolated account stack")
	}
	c := Load()
	c.MCPTimeout = 30 * time.Second
	m, err := NewMCP(context.Background(), c, agentwire.Principal{Subject: "probe", DeviceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}

	if err = m.VerifyGuardrails(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLiveIdentityProfile(t *testing.T) {
	if os.Getenv("STS_ACCOUNT_INTEGRATION") != "1" {
		t.Skip("requires isolated account stack")
	}
	c := Load()
	c.MCPTimeout = 30 * time.Second
	control := func(base, token, path string, body any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", base+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		client := http.Client{Timeout: 15 * time.Second}
		r, e := client.Do(req)
		if e != nil {
			t.Fatal("control unavailable")
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatalf("control %s rejected (%d)", path, r.StatusCode)
		}
		var result map[string]any
		if json.NewDecoder(r.Body).Decode(&result) != nil {
			t.Fatal("invalid control response")
		}
		return result
	}
	identityURL, identityKey := os.Getenv("STS_TEST_IDENTITY_URL"), os.Getenv("STS_TEST_IDENTITY_TOKEN")
	profileURL, profileKey := os.Getenv("STS_TEST_PROFILE_URL"), os.Getenv("STS_TEST_PROFILE_TOKEN")
	login := func(name, display, device string) (*MCP, agentwire.Principal, map[string]any) {
		fixture := control(identityURL, identityKey, "/internal/fixtures", map[string]string{"fixture": name, "subject": name, "email": name + "@example.invalid"})
		control(profileURL, profileKey, "/internal/fixtures", map[string]any{"client_code": fixture["client_code"], "name": display})
		begin := control(identityURL, identityKey, "/auth/begin", map[string]string{"device_id": device})
		prepare := control(identityURL, identityKey, "/auth/prepare", map[string]any{"device_id": device, "transaction_id": begin["transaction_id"]})
		u, _ := url.Parse(prepare["authorization_url"].(string))
		credentials := control(identityURL, identityKey, "/auth/complete", map[string]any{"device_id": device, "transaction_id": begin["transaction_id"], "state": u.Query().Get("state"), "code": "fake-" + name})
		current := control(identityURL, identityKey, "/internal/introspect", map[string]any{"device_id": device, "access_token": credentials["access_token"]})
		raw, _ := json.Marshal(current)
		var p agentwire.Principal
		json.Unmarshal(raw, &p)
		m, e := NewMCP(context.Background(), c, p)
		if e != nil {
			t.Fatal(e)
		}
		return m, p, credentials
	}
	a, pa, credentials := login("alice", "Alice Exemplo", strings.Repeat("a", 64))
	b, pb, _ := login("bruno", "Bruno Exemplo", strings.Repeat("b", 64))
	if pa.ClientCode == pb.ClientCode {
		t.Fatal("identity collision")
	}
	call := func(t *testing.T, m *MCP, name string, payload any, key string, confirmed bool) mcp.Result {
		t.Helper()
		raw, _ := json.Marshal(payload)
		r, e := m.Call(context.Background(), name, raw, key, confirmed)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	read := call(t, a, "profile.get_me", map[string]any{}, "read", false)
	if read.IsError || len(read.Content) != 1 {
		t.Fatal("profile read failed")
	}
	var profile map[string]any
	json.Unmarshal([]byte(read.Content[0].Text), &profile)
	city := os.Getenv("STS_TEST_PRIVACY_CANARY")
	if city == "" {
		city = "Recife"
	}
	payload := map[string]any{"expected_version": profile["version"], "changes": map[string]any{"city": city, "gender": nil}}
	operation := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	unconfirmed := call(t, a, "profile.update_me", payload, operation, false)
	if !unconfirmed.IsError {
		t.Fatal("unconfirmed update accepted")
	}
	updated := call(t, a, "profile.update_me", payload, operation, true)
	if updated.IsError {
		t.Fatalf("confirmed update failed: %+v", updated)
	}
	replay := call(t, a, "profile.update_me", payload, operation, true)
	if !reflect.DeepEqual(updated, replay) {
		t.Fatal("replay differs")
	}
	conflict := call(t, a, "profile.update_me", payload, operation+"-other", true)
	if !conflict.IsError {
		t.Fatal("stale version accepted")
	}
	// Independent MCP sessions race through LiteLLM and PostgreSQL, rather than
	// sharing a client lock that could hide transaction/ledger races.
	peer, err := NewMCP(context.Background(), c, pa)
	if err != nil {
		t.Fatal(err)
	}
	currentProfile := func(t *testing.T) map[string]any {
		t.Helper()
		result := call(t, a, "profile.get_me", map[string]any{}, "current", false)
		var value map[string]any
		if result.IsError || len(result.Content) != 1 || json.Unmarshal([]byte(result.Content[0].Text), &value) != nil {
			t.Fatal("cannot read profile after write")
		}
		return value
	}
	concurrent := func(t *testing.T, payload any, keys [2]string) [2]mcp.Result {
		t.Helper()
		raw, _ := json.Marshal(payload)
		type outcome struct {
			index  int
			result mcp.Result
			err    error
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		start := make(chan struct{})
		finished := make(chan outcome, 2)
		for index, client := range []*MCP{a, peer} {
			go func(index int, client *MCP) {
				<-start
				result, err := client.Call(ctx, "profile.update_me", raw, keys[index], true)
				finished <- outcome{index, result, err}
			}(index, client)
		}
		close(start)
		var results [2]mcp.Result
		for range 2 {
			select {
			case outcome := <-finished:
				if outcome.err != nil {
					t.Fatal("concurrent MCP request failed")
				}
				results[outcome.index] = outcome.result
			case <-ctx.Done():
				t.Fatal("concurrent writes timed out")
			}
		}
		return results
	}
	t.Run("concurrent_replay_commits_once", func(t *testing.T) {
		before := currentProfile(t)["version"].(float64)
		payload := map[string]any{"expected_version": before, "changes": map[string]any{"city": "Curitiba"}}
		key := operation + "-concurrent-replay"
		results := concurrent(t, payload, [2]string{key, key})
		if results[0].IsError || results[1].IsError || !reflect.DeepEqual(results[0], results[1]) {
			t.Fatal("concurrent replay did not return the same committed result")
		}
		if currentProfile(t)["version"] != before+1 {
			t.Fatal("concurrent replay wrote more than once")
		}
	})
	t.Run("competing_versions_have_one_winner", func(t *testing.T) {
		before := currentProfile(t)["version"].(float64)
		payload := map[string]any{"expected_version": before, "changes": map[string]any{"city": "Salvador"}}
		results := concurrent(t, payload, [2]string{operation + "-race-a", operation + "-race-b"})
		if results[0].IsError == results[1].IsError || currentProfile(t)["version"] != before+1 {
			t.Fatal("competing writes must commit exactly one version")
		}
	})
	t.Run("lost_response_retries_without_duplicate_write", func(t *testing.T) {
		before := currentProfile(t)["version"].(float64)
		payload := map[string]any{"expected_version": before, "changes": map[string]any{"city": "Fortaleza"}}
		raw, _ := json.Marshal(payload)
		key := operation + "-lost-response"
		original := a.client.Transport
		transport := original
		if transport == nil {
			transport = http.DefaultTransport
		}
		a.client.Transport = accountRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			response, err := transport.RoundTrip(request)
			if err != nil {
				return nil, err
			}
			// Consume the server response so its transaction has finished, then
			// simulate losing it before the agent receives the outcome.
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			return nil, io.ErrUnexpectedEOF
		})
		_, lost := a.Call(context.Background(), "profile.update_me", raw, key, true)
		a.client.Transport = original
		if lost == nil {
			t.Fatal("response loss was not simulated")
		}
		retried := call(t, a, "profile.update_me", payload, key, true)
		if retried.IsError || currentProfile(t)["version"] != before+1 {
			t.Fatal("retry after lost response must recover the committed result")
		}
		if !reflect.DeepEqual(retried, call(t, a, "profile.update_me", payload, key, true)) {
			t.Fatal("recovered result changed on replay")
		}
	})
	for _, mode := range []string{"ui_approve", "ui_cancel", "voice_approve", "voice_cancel", "wrong_operation", "expired", "changed_arguments"} {
		t.Run("confirmation_"+mode, func(t *testing.T) {
			before := currentProfile(t)["version"].(float64)
			raw, _ := json.Marshal(map[string]any{"expected_version": before, "changes": map[string]any{"city": "Campinas"}})
			session, err := orchestrator.New(context.Background(), a, []string{"profile.update_me"}, operation+mode, policies)
			if err != nil {
				t.Fatal(err)
			}
			tools := &toolRuntime{session: session}
			_, pending := session.Plan("pending", "profile_update_me", "original-turn", raw)
			if pending == nil || session.Pending == nil || currentProfile(t)["version"] != before {
				t.Fatal("update must wait for explicit confirmation without writing")
			}
			var job orchestrator.Job
			var immediate *mcp.Result
			execute := false
			switch mode {
			case "ui_approve", "ui_cancel":
				approved := mode == "ui_approve"
				var accepted bool
				job, immediate, accepted = tools.confirm("pending", approved)
				if !accepted {
					t.Fatal("matching UI confirmation rejected")
				}
				execute = approved
				if !approved {
					event, err := agentEventFromVoice(toolResultEvent(job, *immediate))
					if err != nil || !strings.Contains(string(event.Tool.Result), "cancelled") || session.Pending != nil {
						t.Fatal("UI cancellation must clear pending operation and publish cancelled result")
					}
				}
			case "voice_approve", "voice_cancel":
				if session.ObserveUser("original-turn", "confirmo alterar meus dados") || session.ObserveUser("next-turn", `"confirmo alterar meus dados"`) {
					t.Fatal("same-turn or quoted speech authorized a write")
				}
				if mode == "voice_approve" {
					if !session.ObserveUser("next-turn", "confirmo alterar meus dados") {
						t.Fatal("explicit later-turn approval rejected")
					}
					job, immediate = session.Plan("voice-approved", "profile_update_me", "next-turn", raw)
					execute = true
				} else {
					session.ObserveUser("next-turn", "cancelar")
					if session.Pending != nil {
						t.Fatal("voice cancellation retained pending update")
					}
				}
			case "wrong_operation", "expired":
				id := "other-operation"
				if mode == "expired" {
					session.Pending.Expires = time.Now().Add(-time.Second)
					id = "pending"
				}
				if _, _, accepted := tools.confirm(id, true); accepted {
					t.Fatal("invalid UI confirmation accepted")
				}
			case "changed_arguments":
				session.ObserveUser("next-turn", "confirmo alterar meus dados")
				changed, _ := json.Marshal(map[string]any{"expected_version": before, "changes": map[string]any{"city": "Santos"}})
				_, immediate = session.Plan("changed", "profile_update_me", "next-turn", changed)
				if immediate == nil || session.Pending == nil || session.Pending.Approved {
					t.Fatal("approval carried over to changed arguments")
				}
			}
			if execute {
				if immediate != nil || !job.Confirmed {
					t.Fatal("approved operation was not executable")
				}
				result := session.Execute(context.Background(), job)
				session.Complete(job, result)
				if result.IsError || currentProfile(t)["version"] != before+1 {
					t.Fatal("confirmed write did not commit exactly once")
				}
				if _, _, accepted := tools.confirm("pending", true); accepted {
					t.Fatal("UI approval reused after completion")
				}
			} else if currentProfile(t)["version"] != before {
				t.Fatal("cancelled or invalid confirmation changed profile")
			}
		})
	}
	own := call(t, b, "profile.get_me", map[string]any{}, "read", false)
	if own.IsError || !strings.Contains(own.Content[0].Text, "Bruno Exemplo") {
		t.Fatal("B read incorrect owner")
	}
	forgedPrincipal := pa
	forgedPrincipal.ClientCode = pb.ClientCode
	forged, err := NewMCP(context.Background(), c, forgedPrincipal)
	if err != nil {
		t.Fatal(err)
	}
	if !call(t, forged, "profile.get_me", map[string]any{}, "forged", false).IsError {
		t.Fatal("A read B")
	}
	control(identityURL, identityKey, "/auth/logout", map[string]any{"device_id": pa.DeviceID, "refresh_token": credentials["refresh_token"]})
	if !call(t, a, "profile.get_me", map[string]any{}, "revoked", false).IsError {
		t.Fatal("revoked session read profile")
	}
}

type accountRoundTripFunc func(*http.Request) (*http.Response, error)

func (f accountRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestLiveAccountSessionV2(t *testing.T) {
	if os.Getenv("STS_ACCOUNT_INTEGRATION") != "1" {
		t.Skip("requires isolated account stack")
	}
	cfg := Load()
	cfg.Provider = "fake"
	cfg.MCPTimeout = 30 * time.Second
	handler, err := agentwire.NewHandler(agentwire.ServerConfig{Protocol: agentwire.ProtocolV2, AgentID: "account", Token: "test-service-token", ReadTimeout: time.Second * 15, WriteTimeout: time.Second * 15, IdleTimeout: time.Second * 30, MaxSessions: 2}, &Factory{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	remote, err := agentwire.NewRemoteFactory([]agentwire.Endpoint{{AgentID: "account", Protocol: agentwire.ProtocolV2, URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/v2/session", Token: "test-service-token"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	session, err := remote.Open(ctx, agentwire.AgentSessionStart{Protocol: agentwire.ProtocolV2, SessionID: sessionID, AgentID: "account", Provider: "fake", Principal: agentwire.Principal{Subject: "anonymous-test", DeviceID: strings.Repeat("e", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, expected := range []string{agentwire.AgentSessionReady, agentwire.OutputTranscript, agentwire.ToolConfirmation, "ui.a2ui"} {
		if expected == "ui.a2ui" {
			if err := session.Send(ctx, agentwire.AgentCommand{Type: agentwire.InputConfirmation, SessionID: sessionID, OperationID: welcomeOperation, Approved: true}); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case event := <-session.Events():
			if event.Type != expected {
				t.Fatalf("expected %s, got %s", expected, event.Type)
			}
			if expected == "ui.a2ui" && (!json.Valid(event.UI) || !strings.Contains(string(event.UI), "account.authenticate")) {
				t.Fatal("invalid authentication surface")
			}
		case <-ctx.Done():
			t.Fatal("account session did not publish authentication UI")
		}
	}
}
