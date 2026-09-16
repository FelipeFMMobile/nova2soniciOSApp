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

	"stsmodel.local/poc/internal/config"
	"stsmodel.local/poc/internal/gateway"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	voice, err := gateway.New(cfg, logger)
	if err != nil {
		logger.Error("gateway configuration rejected", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: cfg.GatewayAddress, Handler: voice, ReadHeaderTimeout: cfg.ReadTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	logger.Info("gateway listening", "address", cfg.GatewayAddress, "provider", cfg.Provider)
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("gateway failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := voice.Close(shutdown); err != nil {
			logger.Error("voice shutdown timed out")
		}
		if err := server.Shutdown(shutdown); err != nil {
			logger.Error("http shutdown timed out")
			_ = server.Close()
		}
	}
}
