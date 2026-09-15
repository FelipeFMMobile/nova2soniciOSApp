SHELL := /bin/zsh
GOCACHE ?= /private/tmp/sts-go-cache

.PHONY: build test race gateway mcp fmt check nova-install nova-bridge nova-smoke

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

nova-install:
	python3.12 -m venv services/nova-bridge/.venv
	services/nova-bridge/.venv/bin/pip install -r services/nova-bridge/requirements.txt

nova-bridge:
	PYTHONPATH=services/nova-bridge services/nova-bridge/.venv/bin/python -m nova_bridge.server

nova-smoke:
	services/nova-bridge/.venv/bin/python tests/e2e/nova_realtime_smoke.py "$(WAV)"
