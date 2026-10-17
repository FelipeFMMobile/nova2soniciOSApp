FROM golang:1.27-bookworm AS build
WORKDIR /src
ENV GOWORK=off
COPY packages/go ./packages/go
COPY apps/account-agent ./apps/account-agent
WORKDIR /src/apps/account-agent
RUN go build -trimpath -o /out/account-agent ./cmd/account-agent
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/account-agent /usr/local/bin/account-agent
COPY apps/account-agent/prompts/system.md /app/prompts/system.md
USER 10001:10001
ENV STS_AGENT_PROMPT_FILE=/app/prompts/system.md
ENTRYPOINT ["/usr/local/bin/account-agent"]
