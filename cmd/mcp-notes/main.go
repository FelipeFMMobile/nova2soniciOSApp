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
	path := flag.String("db", "./data/sts.sqlite", "SQLite notes path")
	flag.Parse()
	store, err := storage.Open(*path)
	if err != nil {
		slog.Error("cannot open notes database")
		os.Exit(1)
	}
	defer store.Close()
	if err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, store); err != nil {
		slog.Error("MCP transport closed")
		os.Exit(1)
	}
}
