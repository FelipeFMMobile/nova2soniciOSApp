package main

import (
	"log/slog"
	"os"

	"stsmodel.local/poc/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("gateway bootstrap complete", "address", cfg.GatewayAddress, "provider", cfg.Provider)
}
