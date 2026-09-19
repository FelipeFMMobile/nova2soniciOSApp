FROM golang:1.27-bookworm AS mcp-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=1 go build -trimpath -o /out/mcp-notes ./cmd/mcp-notes && \
    CGO_ENABLED=1 go build -trimpath -o /out/mcp-agenda ./cmd/mcp-agenda

FROM ghcr.io/berriai/litellm-database:v1.101.0@sha256:070df884f52600352a4aedda059180be141f3645c61c59b89b0ee35c17a4ea84
COPY --from=mcp-build /out/mcp-notes /app/bin/mcp-notes
COPY --from=mcp-build /out/mcp-agenda /app/bin/mcp-agenda
COPY deploy/litellm/config.yaml /app/config.yaml
