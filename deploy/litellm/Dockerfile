FROM golang:1.27-bookworm AS mcp-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=1 go build -trimpath -o /out/mcp-notes ./cmd/mcp-notes && \
    CGO_ENABLED=1 go build -trimpath -o /out/mcp-agenda ./cmd/mcp-agenda

# The database image for this release is built with the bedrock-realtime extra
# (aws-sdk-bedrock-runtime[awscrt]). Pin the multi-architecture manifest so a
# rebuild cannot silently change either LiteLLM or its Nova Sonic transport.
FROM ghcr.io/berriai/litellm-database:v1.103.0-dev.2@sha256:d4f41128aac9652c49f7a2954311f9dab28cca61116c3f7c835ae5ead3bd5d39
COPY deploy/litellm/patches/redact_mcp_spend.py /tmp/redact_mcp_spend.py
COPY deploy/litellm/patches/bedrock_realtime_turn_detection.py /tmp/bedrock_realtime_turn_detection.py
RUN python /tmp/redact_mcp_spend.py && \
    python /tmp/bedrock_realtime_turn_detection.py && \
    rm /tmp/redact_mcp_spend.py /tmp/bedrock_realtime_turn_detection.py
COPY --from=mcp-build /out/mcp-notes /app/bin/mcp-notes
COPY --from=mcp-build /out/mcp-agenda /app/bin/mcp-agenda
COPY deploy/litellm/config.yaml /app/config.yaml
