SHELL := /bin/zsh
GOCACHE ?= /private/tmp/sts-go-cache

.PHONY: build test race gateway mcp fmt check qwen-config qwen-smoke

build:
	env GOCACHE=$(GOCACHE) go build ./...

test:
	env GOCACHE=$(GOCACHE) go test ./...

race:
	env GOCACHE=$(GOCACHE) go test -race ./...

gateway:
	env GOCACHE=$(GOCACHE) go run ./cmd/gateway

mcp:
	env GOCACHE=$(GOCACHE) go run ./cmd/mcp-notes

fmt:
	gofmt -w cmd internal tests

check: build test race

qwen-config:
	docker compose -f deploy/runpod/compose.yaml config --quiet

qwen-smoke:
	python3 tests/e2e/qwen_realtime_smoke.py "$(WAV)"
