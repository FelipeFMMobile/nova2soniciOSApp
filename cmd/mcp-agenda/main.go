package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strconv"

	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/storage"
)

func main() {
	path := flag.String("db", "./data/agenda.sqlite", "SQLite fictitious agenda path")
	fixtures := flag.Bool("fixtures", false, "seed deterministic fictitious appointments")
	mode := flag.String("context-mode", "meta", "host context transport: meta or envelope")
	flag.Parse()
	if raw := os.Getenv("STS_AGENDA_FIXTURES"); raw != "" {
		enabled, parseErr := strconv.ParseBool(raw)
		if raw == "enabled" || raw == "disabled" {
			enabled, parseErr = raw == "enabled", nil
		}
		if parseErr != nil {
			slog.Error("invalid agenda fixtures configuration")
			os.Exit(1)
		}
		*fixtures = *fixtures || enabled
	}
	codec, err := mcp.ServerEnvelope(*mode, "local")
	if err != nil {
		slog.Error("invalid MCP context configuration")
		os.Exit(1)
	}
	s, err := storage.OpenAgenda(*path)
	if err != nil {
		slog.Error("cannot open agenda database")
		os.Exit(1)
	}
	defer s.Close()
	if *fixtures {
		if err = s.SeedFixtures(context.Background()); err != nil {
			slog.Error("cannot seed fixtures")
			os.Exit(1)
		}
	}
	if err = mcp.ServeAgenda(context.Background(), os.Stdin, os.Stdout, s, codec); err != nil {
		slog.Error("MCP transport closed")
		os.Exit(1)
	}
}
