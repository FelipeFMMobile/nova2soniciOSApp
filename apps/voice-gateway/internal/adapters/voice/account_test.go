package gateway

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stsmodel.local/voice-gateway/internal/bootstrap/config"
)

func TestAccountTransportRequiresTLSOrLoopback(t *testing.T) {
	for _, tc := range []struct {
		remote  string
		tls     bool
		allowed bool
	}{
		{"127.0.0.1:1000", false, true}, {"[::1]:1000", false, true},
		{"192.0.2.1:1000", false, false}, {"192.0.2.1:1000", true, true},
	} {
		r := httptest.NewRequest("POST", "http://gateway/v2/auth/begin", strings.NewReader(`{}`))
		r.RemoteAddr = tc.remote
		if tc.tls {
			r.TLS = &tls.ConnectionState{}
		}
		if secureAccountRequest(r) != tc.allowed {
			t.Fatalf("unexpected policy for %s", tc.remote)
		}
	}
}

func TestAccountControlRejectsIdentityInjectionAndBindsDevice(t *testing.T) {
	calls := 0
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer service-only" {
			t.Error("missing service credential")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid forwarded body")
		}
		if len(body) != 1 || len(body["device_id"]) != 64 || body["device_id"] == strings.Repeat("a", 64) {
			t.Error("device was not hashed by gateway")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transaction_id":"opaque"}`))
	}))
	defer identity.Close()
	server := &Server{cfg: config.Config{IdentityURL: identity.URL, IdentityToken: "service-only"}}
	for _, body := range []string{`{"device_id":"forged"}`, `{"subject":"other"}`, `{} {}`, `null`} {
		r := httptest.NewRequest("POST", "http://gateway/v2/auth/begin", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1000"
		r.Header.Set("X-STS-Device", strings.Repeat("a", 64))
		response := httptest.NewRecorder()
		server.accountControl(response, r)
		if response.Code != 400 {
			t.Fatalf("injected body accepted: %d", response.Code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid input reached identity")
	}
	r := httptest.NewRequest("POST", "http://gateway/v2/auth/begin", strings.NewReader(`{}`))
	r.RemoteAddr = "127.0.0.1:1000"
	r.Header.Set("X-STS-Device", strings.Repeat("a", 64))
	response := httptest.NewRecorder()
	server.accountControl(response, r)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || calls != 1 {
		t.Fatal("control flow failed")
	}
}
