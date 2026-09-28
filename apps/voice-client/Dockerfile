# Build from the repository root: docker build -f apps/voice-client/Dockerfile .
FROM golang:1.27-bookworm AS build
WORKDIR /src
ENV GOWORK=off
COPY packages/go ./packages/go
COPY apps/voice-client ./apps/voice-client
WORKDIR /src/apps/voice-client
RUN CGO_ENABLED=1 go build -trimpath -o /out/app ./cmd/voice-client
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN chown 10001:10001 /app
COPY --from=build /out/app /usr/local/bin/voice-client
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/voice-client"]
