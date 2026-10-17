package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"stsmodel.local/agentwire"
	"time"
)

func secureAccountRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return r.TLS != nil || err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}
func deviceIdentity(r *http.Request) (string, error) {
	device := r.Header.Get("X-STS-Device")
	if len(device) < 43 || len(device) > 128 || strings.ContainsAny(device, " \r\n\t") {
		return "", errors.New("device binding required")
	}
	sum := sha256.Sum256([]byte(device))
	return hex.EncodeToString(sum[:]), nil
}
func (s *Server) identityRequest(ctx context.Context, path string, body any) ([]byte, error) {
	if s.cfg.IdentityURL == "" || s.cfg.IdentityToken == "" {
		return nil, errors.New("account unavailable")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.IdentityURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.IdentityToken)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("identity unavailable")
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, 32769))
	if err != nil || len(data) > 32768 || resp.StatusCode != 200 {
		return nil, errors.New("identity rejected operation")
	}
	return data, nil
}
func (s *Server) accountPrincipal(r *http.Request) (agentwire.Principal, error) {
	if !secureAccountRequest(r) {
		return agentwire.Principal{}, errors.New("TLS required")
	}
	device, err := deviceIdentity(r)
	if err != nil {
		return agentwire.Principal{}, err
	}
	token := r.Header.Get("X-STS-Account")
	if token == "" {
		return agentwire.Principal{Subject: "anon-" + device[:24], DeviceID: device}, nil
	}
	if len(token) > 256 {
		return agentwire.Principal{}, errors.New("invalid account token")
	}
	data, err := s.identityRequest(r.Context(), "/internal/introspect", map[string]string{"access_token": token, "device_id": device})
	if err != nil {
		return agentwire.Principal{}, err
	}
	var principal agentwire.Principal
	if json.Unmarshal(data, &principal) != nil || !principal.Authenticated || principal.DeviceID != device || agentwire.ValidatePrincipal(principal) != nil {
		return principal, errors.New("invalid identity response")
	}
	return principal, nil
}
func (s *Server) accountControl(w http.ResponseWriter, r *http.Request) {
	if !secureAccountRequest(r) {
		writeJSON(w, 426, map[string]string{"error": "TLS required"})
		return
	}
	device, err := deviceIdentity(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "device binding required"})
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/v2/auth/")
	allowed := map[string][]string{"begin": {}, "prepare": {"transaction_id"}, "complete": {"transaction_id", "state", "code"}, "refresh": {"refresh_token"}, "logout": {"refresh_token"}}
	fields, ok := allowed[action]
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body map[string]string
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	if decoder.Decode(&body) != nil || body == nil || decoder.Decode(new(any)) != io.EOF || len(body) != len(fields) {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	for _, key := range fields {
		if body[key] == "" || len(body[key]) > 4096 {
			writeJSON(w, 400, map[string]string{"error": "invalid request"})
			return
		}
	}
	body["device_id"] = device
	data, err := s.identityRequest(r.Context(), "/auth/"+action, body)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "authentication_failed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
