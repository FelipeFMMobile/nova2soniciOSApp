package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stsmodel.local/agentwire"
	agentconfig "stsmodel.local/personal-assistant-agent/config"
	"stsmodel.local/personal-assistant-agent/inprocess"
)

func main() {
	config, err := agentconfig.LoadService()
	if err != nil {
		slog.Error("invalid personal assistant configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	factory, err := inprocess.New(config.Runtime, logger, inprocess.Hooks{}, nil)
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
	server := &http.Server{Addr: config.Address, Handler: handler, ReadHeaderTimeout: config.ReadTimeout}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
