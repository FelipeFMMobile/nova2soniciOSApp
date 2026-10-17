package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"stsmodel.local/account-agent/internal/account"
	"stsmodel.local/agentwire"
	"syscall"
	"time"
)

func main() {
	cfg := account.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	address := os.Getenv("STS_AGENT_ADDRESS")
	if address == "" {
		address = "127.0.0.1:8091"
	}
	if !agentwire.IsLoopbackAddress(address) {
		logger.Error("account agent must listen on loopback")
		os.Exit(1)
	}
	if cfg.Validate() != nil {
		logger.Error("invalid account runtime configuration")
		os.Exit(1)
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	probe, probeErr := account.NewMCP(probeCtx, cfg, agentwire.Principal{Subject: "readiness", DeviceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if probeErr == nil {
		probeErr = probe.VerifyGuardrails(probeCtx)
	}
	probeCancel()
	if probeErr != nil {
		logger.Error("account readiness failed: MCP guardrails required")
		os.Exit(1)
	}
	handler, err := agentwire.NewHandler(agentwire.ServerConfig{Protocol: agentwire.ProtocolV2, AgentID: "account", Token: os.Getenv("STS_AGENT_TOKEN"), ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxSessions: 16}, &account.Factory{Config: cfg, Logger: logger})
	if err != nil {
		logger.Error("invalid account configuration")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.Handle("/v2/session", handler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("account server failed")
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
