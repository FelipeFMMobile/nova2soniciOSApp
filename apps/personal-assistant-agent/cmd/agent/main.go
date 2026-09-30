package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stsmodel.local/agentwire"
	agentconfig "stsmodel.local/personal-assistant-agent/config"
	"stsmodel.local/personal-assistant-agent/internal/fake"
	"stsmodel.local/personal-assistant-agent/session"
)

func main() {
	config, err := agentconfig.LoadService()
	if err != nil {
		slog.Error("invalid personal assistant configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	var factory agentwire.AgentSessionFactory
	if config.Runtime.Provider == "fake" {
		factory = fake.New(config.Runtime.TurnTimeout, logger)
	} else {
		factory, err = session.New(config.Runtime, logger, session.Hooks{}, nil)
	}
	if err != nil {
		logger.Error("personal assistant runtime rejected", "error", err)
		os.Exit(1)
	}
	handler, err := agentwire.NewHandler(agentwire.ServerConfig{
		AgentID: config.AgentID, Token: config.Token, ReadTimeout: config.ReadTimeout,
		WriteTimeout: config.WriteTimeout, IdleTimeout: config.IdleTimeout, MaxSessions: config.MaxSessions,
	}, factory)
	if err != nil {
		logger.Error("Agent Session configuration rejected", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.Handle("/v1/session", handler)
	server := &http.Server{Addr: config.Address, Handler: mux, ReadHeaderTimeout: config.ReadTimeout,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	logger.Info("personal assistant listening", "address", config.Address, "agent_id", config.AgentID, "protocol", agentwire.Protocol)
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("personal assistant failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if server.Shutdown(shutdown) != nil {
			_ = server.Close()
		}
	}
}
