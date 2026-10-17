SHELL := /bin/zsh
GOCACHE ?= /private/tmp/sts-go-cache
NOVA_GATEWAY_OUTPUT ?= /private/tmp/sts-nova-gateway-response.wav
NOVA_BARGE_OUTPUT ?= /private/tmp/sts-nova-gateway-barge-response.wav
APPLE_PROJECT := apps/apple/NovaVoice.xcodeproj
APPLE_BUILD_ROOT ?= /private/tmp/sts-apple
APPLE_SIMULATOR_ID ?= 599499E4-DD38-4C6C-9459-D4FDA8B2AE43

.PHONY: build test race vet gateway agent mcp mcp-build agenda-build mcp-gateway fmt check voice-demo voice-cancel nova-demo nova-barge-demo
.PHONY: apple-core-test apple-build-macos apple-build-ios apple-ui-test
.PHONY: dev dev-plan dev-test architecture-diagram
.PHONY: java-test account
.PHONY: account-smoke account-ui-test
.PHONY: litellm-up litellm-down litellm-privacy-audit litellm-redact-history

litellm-up:
	docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml up --build -d

litellm-down:
	docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml down

litellm-privacy-audit:
	docker exec -i litellm-postgres-1 sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < deploy/litellm/audit-spend-privacy.sql

litellm-redact-history:
	docker exec -i litellm-postgres-1 sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' < deploy/litellm/redact-spend-history.sql

dev:
	python3 scripts/dev.py

dev-plan:
	python3 scripts/dev.py --dry-run

dev-test:
	python3 -m unittest discover -s tests/scripts -v

java-test:
	cd apps/mcp-identity && ./mvnw -B verify
	cd apps/mcp-profile && ./mvnw -B verify

account:
	env GOCACHE=$(GOCACHE) go run ./apps/account-agent/cmd/account-agent

account-smoke:
	python3 scripts/account_smoke.py

account-ui-test:
	python3 scripts/account_smoke.py --apple-ui --simulator $(APPLE_SIMULATOR_ID)

architecture-diagram:
	mkdir -p /private/tmp/sts-swift-cache docs/images
	env SWIFT_MODULECACHE_PATH=/private/tmp/sts-swift-cache CLANG_MODULE_CACHE_PATH=/private/tmp/sts-swift-cache swift scripts/render_architecture.swift docs/images/architecture-litellm.png

apple-core-test:
	swift test --package-path apps/apple/VoiceCore --scratch-path $(APPLE_BUILD_ROOT)-core

apple-build-macos:
	xcodebuild -project $(APPLE_PROJECT) -scheme NovaVoice -destination 'platform=macOS,arch=arm64' -configuration Debug -derivedDataPath $(APPLE_BUILD_ROOT)-macos build -quiet

apple-build-ios:
	xcodebuild -project $(APPLE_PROJECT) -scheme NovaVoice -destination 'generic/platform=iOS Simulator' -configuration Debug -derivedDataPath $(APPLE_BUILD_ROOT)-ios CODE_SIGNING_ALLOWED=NO build -quiet

apple-ui-test:
	xcodebuild -project $(APPLE_PROJECT) -scheme NovaVoice -destination 'platform=iOS Simulator,id=$(APPLE_SIMULATOR_ID)' -configuration Debug -derivedDataPath $(APPLE_BUILD_ROOT)-ios -parallel-testing-enabled NO test -quiet

build:
	env GOCACHE=$(GOCACHE) python3 scripts/modules.py build

test:
	env GOCACHE=$(GOCACHE) python3 scripts/modules.py test

race:
	env GOCACHE=$(GOCACHE) python3 scripts/modules.py race

vet:
	env GOCACHE=$(GOCACHE) python3 scripts/modules.py vet

gateway:
	env GOCACHE=$(GOCACHE) go run ./apps/voice-gateway/cmd/gateway

agent:
	env GOCACHE=$(GOCACHE) go run ./apps/personal-assistant-agent/cmd/agent

mcp:
	env GOCACHE=$(GOCACHE) go run ./apps/mcp-notes/cmd/mcp-notes

mcp-build:
	env GOCACHE=$(GOCACHE) go build -o bin/mcp-notes ./apps/mcp-notes/cmd/mcp-notes

agenda-build:
	env GOCACHE=$(GOCACHE) go build -o bin/mcp-agenda ./apps/mcp-agenda/cmd/mcp-agenda

mcp-gateway:
	python3 scripts/dev.py --mcps notes

fmt:
	gofmt -w apps/voice-gateway apps/personal-assistant-agent apps/account-agent apps/voice-client apps/mcp-notes apps/mcp-agenda packages/go tests

check: build test race vet isolated

voice-demo:
	env GOCACHE=$(GOCACHE) go run ./apps/voice-client/cmd/voice-client

voice-cancel:
	env GOCACHE=$(GOCACHE) go run ./apps/voice-client/cmd/voice-client -cancel-after 120ms

nova-demo:
	env GOCACHE=$(GOCACHE) go run ./apps/voice-client/cmd/voice-client -provider nova -wav "$(WAV)" -output "$(NOVA_GATEWAY_OUTPUT)"

nova-barge-demo:
	env GOCACHE=$(GOCACHE) go run ./apps/voice-client/cmd/voice-client -provider nova -wav "$(WAV)" -barge-wav "$(BARGE_WAV)" -output "$(NOVA_BARGE_OUTPUT)"

# Local, deterministic rollout gates; no AWS inference.
agent-parity:
	GOWORK=off GOCACHE=$(GOCACHE) go test ./tests/voice -count=1

agent-latency:
	cd packages/go/agentwire && GOWORK=off GOCACHE=$(GOCACHE) STS_AGENT_LATENCY=1 go test -run TestLocalTransportLatency -count=1 -v

agent-clients: build
	GOWORK=off GOCACHE=$(GOCACHE) STS_ROLLOUT_APPLE=1 go test ./tests/e2e -run TestAgentRolloutProcesses -count=1 -v

.PHONY: isolated images agent-parity agent-latency agent-clients
isolated:
	env GOCACHE=$(GOCACHE) python3 scripts/modules.py isolated

images:
	docker build -f apps/voice-gateway/Dockerfile -t sts-voice-gateway:local .
	docker build -f apps/personal-assistant-agent/Dockerfile -t sts-personal-assistant-agent:local .
