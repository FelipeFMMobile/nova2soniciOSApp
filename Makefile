SHELL := /bin/zsh
GOCACHE ?= /private/tmp/sts-go-cache
NOVA_OUTPUT ?= /private/tmp/sts-nova-response.wav
NOVA_GATEWAY_OUTPUT ?= /private/tmp/sts-nova-gateway-response.wav
NOVA_BARGE_OUTPUT ?= /private/tmp/sts-nova-gateway-barge-response.wav

.PHONY: build test race vet gateway mcp mcp-build mcp-gateway fmt check voice-demo voice-cancel nova-install nova-test nova-bridge nova-smoke nova-demo nova-barge-demo

build:
	env GOCACHE=$(GOCACHE) go build ./...

test:
	env GOCACHE=$(GOCACHE) go test ./...

race:
	env GOCACHE=$(GOCACHE) go test -race ./...

vet:
	env GOCACHE=$(GOCACHE) go vet ./...

gateway:
	env GOCACHE=$(GOCACHE) go run ./cmd/gateway

mcp:
	env GOCACHE=$(GOCACHE) go run ./cmd/mcp-notes

mcp-build:
	env GOCACHE=$(GOCACHE) go build -o bin/mcp-notes ./cmd/mcp-notes

mcp-gateway: mcp-build
	env GOCACHE=$(GOCACHE) STS_PROVIDER=nova STS_MCP_COMMAND="$(CURDIR)/bin/mcp-notes" STS_MCP_ARGS='["-db","$(CURDIR)/data/sts.sqlite"]' go run ./cmd/gateway

fmt:
	gofmt -w cmd internal tests

check: build test race vet

voice-demo:
	env GOCACHE=$(GOCACHE) go run ./cmd/voice-client

voice-cancel:
	env GOCACHE=$(GOCACHE) go run ./cmd/voice-client -cancel-after 120ms

nova-install:
	python3.12 -m venv services/nova-bridge/.venv
	services/nova-bridge/.venv/bin/pip install -r services/nova-bridge/requirements.txt

nova-test:
	PYTHONPATH=services/nova-bridge services/nova-bridge/.venv/bin/python -m unittest discover -s tests/python -v

nova-bridge:
	PYTHONPATH=services/nova-bridge services/nova-bridge/.venv/bin/python -m nova_bridge.server

nova-smoke:
	services/nova-bridge/.venv/bin/python tests/e2e/nova_realtime_smoke.py "$(WAV)" --output "$(NOVA_OUTPUT)"

nova-demo:
	env GOCACHE=$(GOCACHE) go run ./cmd/voice-client -provider nova -wav "$(WAV)" -output "$(NOVA_GATEWAY_OUTPUT)"

nova-barge-demo:
	env GOCACHE=$(GOCACHE) go run ./cmd/voice-client -provider nova -wav "$(WAV)" -barge-wav "$(BARGE_WAV)" -output "$(NOVA_BARGE_OUTPUT)"
