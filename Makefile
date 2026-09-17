SHELL := /bin/zsh
GOCACHE ?= /private/tmp/sts-go-cache
NOVA_OUTPUT ?= /private/tmp/sts-nova-response.wav
NOVA_GATEWAY_OUTPUT ?= /private/tmp/sts-nova-gateway-response.wav
NOVA_BARGE_OUTPUT ?= /private/tmp/sts-nova-gateway-barge-response.wav
APPLE_PROJECT := apps/apple/NovaVoice.xcodeproj
APPLE_BUILD_ROOT ?= /private/tmp/sts-apple
APPLE_SIMULATOR_ID ?= 599499E4-DD38-4C6C-9459-D4FDA8B2AE43

.PHONY: build test race vet gateway mcp mcp-build agenda-build mcp-gateway fmt check voice-demo voice-cancel nova-install nova-test nova-bridge nova-smoke nova-demo nova-barge-demo
.PHONY: apple-core-test apple-build-macos apple-build-ios apple-ui-test
.PHONY: dev dev-plan dev-test

dev:
	python3 scripts/dev.py

dev-plan:
	python3 scripts/dev.py --dry-run

dev-test:
	python3 -m unittest discover -s tests/scripts -v

apple-core-test:
	swift test --package-path apps/apple/VoiceCore --scratch-path $(APPLE_BUILD_ROOT)-core

apple-build-macos:
	xcodebuild -project $(APPLE_PROJECT) -scheme NovaVoice -destination 'platform=macOS,arch=arm64' -configuration Debug -derivedDataPath $(APPLE_BUILD_ROOT)-macos build -quiet

apple-build-ios:
	xcodebuild -project $(APPLE_PROJECT) -scheme NovaVoice -destination 'generic/platform=iOS Simulator' -configuration Debug -derivedDataPath $(APPLE_BUILD_ROOT)-ios CODE_SIGNING_ALLOWED=NO build -quiet

apple-ui-test:
	xcodebuild -project $(APPLE_PROJECT) -scheme NovaVoice -destination 'platform=iOS Simulator,id=$(APPLE_SIMULATOR_ID)' -configuration Debug -derivedDataPath $(APPLE_BUILD_ROOT)-ios -parallel-testing-enabled NO test -quiet

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

agenda-build:
	env GOCACHE=$(GOCACHE) go build -o bin/mcp-agenda ./cmd/mcp-agenda

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
