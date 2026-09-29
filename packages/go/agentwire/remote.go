package agentwire

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	protocolHeader = "X-STS-Agent-Protocol"
	nonceHeader    = "X-STS-Agent-Nonce"
	proofHeader    = "X-STS-Agent-Proof"
)

func serverProof(token, agentID, nonce string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(Protocol + "\x00" + agentID + "\x00" + nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

// clientCredential is deliberately distinct from the server proof key. A process
// occupying the configured port must not learn the key needed to impersonate it.
func clientCredential(token, agentID string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(Protocol + "\x00client-bearer\x00" + agentID))
	return hex.EncodeToString(mac.Sum(nil))
}

type Endpoint struct {
	AgentID string
	URL     string
	Token   string
}

func ValidateEndpoint(endpoint Endpoint) error {
	if !identifier.MatchString(endpoint.AgentID) || endpoint.Token == "" {
		return errors.New("agent endpoint requires trusted identity and token")
	}
	parsed, err := url.Parse(endpoint.URL)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return errors.New("invalid agent endpoint URL")
	}
	host := parsed.Hostname()
	if parsed.Scheme == "ws" && host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("unencrypted Agent Session endpoint must be loopback")
		}
	}
	return nil
}

type RemoteFactory struct {
	endpoints map[string]Endpoint
	dialer    *websocket.Dialer
}

func NewRemoteFactory(endpoints []Endpoint) (*RemoteFactory, error) {
	f := &RemoteFactory{endpoints: make(map[string]Endpoint), dialer: &websocket.Dialer{HandshakeTimeout: 5 * time.Second}}
	if len(endpoints) == 0 || len(endpoints) > 16 {
		return nil, errors.New("agent catalog requires one to sixteen endpoints")
	}
	for _, endpoint := range endpoints {
		if err := ValidateEndpoint(endpoint); err != nil {
			return nil, err
		}
		if _, duplicate := f.endpoints[endpoint.AgentID]; duplicate {
			return nil, errors.New("duplicate agent identity")
		}
		f.endpoints[endpoint.AgentID] = endpoint
	}
	return f, nil
}

func (f *RemoteFactory) Open(ctx context.Context, start AgentSessionStart) (AgentSession, error) {
	endpoint, ok := f.endpoints[start.AgentID]
	if !ok {
		return nil, errors.New("unknown agent identity")
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+clientCredential(endpoint.Token, endpoint.AgentID))
	headers.Set(protocolHeader, Protocol)
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, errors.New("agent authentication challenge failed")
	}
	nonce := hex.EncodeToString(nonceBytes)
	headers.Set(nonceHeader, nonce)
	connection, response, err := f.dialer.DialContext(ctx, endpoint.URL, headers)
	if err != nil {
		if response != nil {
			return nil, errors.New("agent endpoint rejected session")
		}
		return nil, errors.New("agent endpoint unavailable")
	}
	if response == nil || subtle.ConstantTimeCompare([]byte(response.Header.Get(proofHeader)), []byte(serverProof(endpoint.Token, endpoint.AgentID, nonce))) != 1 {
		_ = connection.Close()
		return nil, errors.New("agent endpoint identity mismatch")
	}
	fail := func(err error) (AgentSession, error) {
		_ = connection.Close()
		return nil, err
	}
	connection.SetReadLimit(MaxMessageBytes)
	_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := connection.WriteJSON(Message{Protocol: Protocol, Kind: OpenSession, AgentID: start.AgentID, Start: &start}); err != nil {
		return fail(errors.New("agent session open failed"))
	}
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	kind, data, err := connection.ReadMessage()
	if err != nil || kind != websocket.TextMessage {
		return fail(errors.New("agent session handshake failed"))
	}
	message, err := Decode(data)
	if err != nil || message.ValidateAccepted(start.AgentID) != nil {
		return fail(errors.New("agent session protocol mismatch"))
	}
	_ = connection.SetReadDeadline(time.Time{})
	s := &remoteSession{connection: connection, sessionID: start.SessionID, events: make(chan AgentEvent, RemoteQueueDepth), stop: make(chan struct{}), done: make(chan struct{})}
	go s.read()
	return s, nil
}

type remoteSession struct {
	connection *websocket.Conn
	sessionID  string
	events     chan AgentEvent
	stop       chan struct{}
	done       chan struct{}
	writeMu    sync.Mutex
	closeOnce  sync.Once
	expected   atomic.Bool
}

func (s *remoteSession) read() {
	defer close(s.done)
	defer s.connection.Close()
	defer close(s.events)
	for {
		kind, data, err := s.connection.ReadMessage()
		if err != nil {
			if !s.expected.Load() {
				select {
				case s.events <- AgentEvent{Type: AgentError, Code: "agent_disconnected", Message: "agent session disconnected"}:
				default:
				}
			}
			return
		}
		if kind != websocket.TextMessage {
			select {
			case s.events <- AgentEvent{Type: AgentError, Code: "agent_protocol_error", Message: "agent returned a non-text frame"}:
			default:
			}
			return
		}
		message, err := Decode(data)
		if err != nil || message.ValidateEvent() != nil {
			select {
			case s.events <- AgentEvent{Type: AgentError, Code: "agent_protocol_error", Message: "agent returned an invalid event"}:
			default:
			}
			return
		}
		select {
		case s.events <- *message.Event:
		case <-s.stop:
			return
		}
	}
}

func (s *remoteSession) Send(ctx context.Context, command AgentCommand) error {
	message := Message{Protocol: Protocol, Kind: SessionCommand, Command: &command}
	if err := message.ValidateCommand(s.sessionID); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.done:
		return errors.New("agent session closed")
	default:
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		deadline = time.Now().Add(5 * time.Second)
	}
	_ = s.connection.SetWriteDeadline(deadline)
	if err := s.connection.WriteJSON(message); err != nil {
		return errors.New("agent session write failed")
	}
	return nil
}

func (s *remoteSession) Events() <-chan AgentEvent { return s.events }

func (s *remoteSession) Close() error {
	s.closeOnce.Do(func() {
		s.expected.Store(true)
		close(s.stop)
		s.writeMu.Lock()
		_ = s.connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session closed"), time.Now().Add(time.Second))
		_ = s.connection.Close()
		s.writeMu.Unlock()
	})
	<-s.done
	return nil
}

type ServerConfig struct {
	AgentID      string
	Token        string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxSessions  int
}

type Handler struct {
	config  ServerConfig
	factory AgentSessionFactory
	slots   chan struct{}
}

func NewHandler(config ServerConfig, factory AgentSessionFactory) (*Handler, error) {
	if !identifier.MatchString(config.AgentID) || config.Token == "" || factory == nil || config.ReadTimeout <= 0 || config.WriteTimeout <= 0 || config.IdleTimeout <= 0 || config.MaxSessions <= 0 || config.MaxSessions > MaxRemoteSessions {
		return nil, errors.New("invalid Agent Session server configuration")
	}
	return &Handler{config: config, factory: factory, slots: make(chan struct{}, config.MaxSessions)}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1alpha1/session" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+clientCredential(h.config.Token, h.config.AgentID))) != 1 || r.Header.Get(protocolHeader) != Protocol {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	nonce := r.Header.Get(nonceHeader)
	decodedNonce, nonceErr := hex.DecodeString(nonce)
	if nonceErr != nil || len(decodedNonce) != 32 {
		http.Error(w, "invalid authentication challenge", http.StatusUnauthorized)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		http.Error(w, "agent session capacity reached", http.StatusServiceUnavailable)
		return
	}
	responseHeaders := http.Header{}
	responseHeaders.Set(proofHeader, serverProof(h.config.Token, h.config.AgentID, nonce))
	responseHeaders.Set(protocolHeader, Protocol)
	connection, err := (&websocket.Upgrader{HandshakeTimeout: h.config.ReadTimeout, CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "" }}).Upgrade(w, r, responseHeaders)
	if err != nil {
		return
	}
	defer connection.Close()
	connection.SetReadLimit(MaxMessageBytes)
	_ = connection.SetReadDeadline(time.Now().Add(h.config.ReadTimeout))
	kind, data, err := connection.ReadMessage()
	if err != nil || kind != websocket.TextMessage {
		return
	}
	message, err := Decode(data)
	if err != nil || message.ValidateOpen() != nil || message.AgentID != h.config.AgentID {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid session open"), time.Now().Add(h.config.WriteTimeout))
		return
	}
	session, err := h.factory.Open(r.Context(), *message.Start)
	if err != nil {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "agent rejected session"), time.Now().Add(h.config.WriteTimeout))
		return
	}
	defer session.Close()
	_ = connection.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
	if err := connection.WriteJSON(Message{Protocol: Protocol, Kind: SessionAccepted, AgentID: h.config.AgentID}); err != nil {
		return
	}
	_ = connection.SetReadDeadline(time.Now().Add(h.config.IdleTimeout))
	commands := make(chan AgentCommand, RemoteQueueDepth)
	readError := make(chan error, 1)
	go h.readCommands(r.Context(), connection, message.Start.SessionID, commands, readError)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-readError:
			return
		case command := <-commands:
			if err := session.Send(r.Context(), command); err != nil {
				return
			}
		case event, ok := <-session.Events():
			if !ok {
				return
			}
			wire := Message{Protocol: Protocol, Kind: SessionEvent, Event: &event}
			if wire.ValidateEvent() != nil {
				return
			}
			_ = connection.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
			if connection.WriteJSON(wire) != nil {
				return
			}
			if event.Type == AgentSessionClosed {
				return
			}
		}
	}
}

func (h *Handler) readCommands(ctx context.Context, connection *websocket.Conn, sessionID string, commands chan<- AgentCommand, failed chan<- error) {
	for {
		kind, data, err := connection.ReadMessage()
		if err != nil || kind != websocket.TextMessage {
			failed <- errors.New("agent command stream closed")
			return
		}
		_ = connection.SetReadDeadline(time.Now().Add(h.config.IdleTimeout))
		message, err := Decode(data)
		if err != nil || message.ValidateCommand(sessionID) != nil {
			failed <- errors.New("invalid agent command")
			return
		}
		select {
		case commands <- *message.Command:
		case <-ctx.Done():
			return
		}
	}
}

func IsLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
