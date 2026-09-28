package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	adapter "stsmodel.local/mcp-notes/internal/adapters/mcp"
	storage "stsmodel.local/mcp-notes/internal/adapters/sqlite"
	"stsmodel.local/mcpwire"
)

func main() {
	path := flag.String("db", "./data/sts.sqlite", "SQLite notes path")
	mode := flag.String("context-mode", "meta", "host context transport: meta or envelope")
	flag.Parse()
	codec, err := mcp.ServerEnvelope(*mode, "memo")
	if err != nil {
		slog.Error("invalid MCP context configuration")
		os.Exit(1)
	}
	store, err := storage.Open(*path)
	if err != nil {
		slog.Error("cannot open notes database")
		os.Exit(1)
	}
	defer store.Close()
	if err := adapter.Serve(context.Background(), os.Stdin, os.Stdout, store, codec); err != nil {
		slog.Error("MCP transport closed")
		os.Exit(1)
	}
}
