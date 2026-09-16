package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"stsmodel.local/poc/internal/mcp"
	"stsmodel.local/poc/internal/storage"
)

func main() {
	path := flag.String("db", "./data/agenda.sqlite", "SQLite fictitious agenda path")
	fixtures := flag.Bool("fixtures", false, "seed deterministic fictitious appointments")
	flag.Parse()
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
	if err = mcp.ServeAgenda(context.Background(), os.Stdin, os.Stdout, s); err != nil {
		slog.Error("MCP transport closed")
		os.Exit(1)
	}
}
