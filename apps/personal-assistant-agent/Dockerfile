# Build from the repository root: docker build -f apps/personal-assistant-agent/Dockerfile .
FROM golang:1.27-bookworm AS build
WORKDIR /src
ENV GOWORK=off
COPY packages/go ./packages/go
COPY apps/personal-assistant-agent ./apps/personal-assistant-agent
WORKDIR /src/apps/personal-assistant-agent
RUN CGO_ENABLED=1 go build -trimpath -o /out/app ./cmd/agent

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN chown 10001:10001 /app
COPY --from=build /out/app /usr/local/bin/personal-assistant-agent
COPY --from=build /src/apps/personal-assistant-agent/prompts/system.md /app/prompts/system.md
USER 10001:10001
ENV STS_AGENT_PROMPT_FILE=/app/prompts/system.md
ENTRYPOINT ["/usr/local/bin/personal-assistant-agent"]
