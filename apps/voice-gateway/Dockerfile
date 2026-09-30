# Build from the repository root: docker build -f apps/voice-gateway/Dockerfile .
FROM golang:1.27-bookworm AS build
WORKDIR /src
ENV GOWORK=off
COPY packages/go ./packages/go
COPY apps/voice-gateway ./apps/voice-gateway
WORKDIR /src/apps/voice-gateway
RUN CGO_ENABLED=1 go build -trimpath -o /out/app ./cmd/gateway
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN chown 10001:10001 /app
COPY --from=build /out/app /usr/local/bin/gateway
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/gateway"]
