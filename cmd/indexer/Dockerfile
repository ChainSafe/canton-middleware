# Build stage
# Using full golang image (not alpine) for toolchain auto-download support
FROM golang:1.23 AS builder

WORKDIR /app

# Allow Go toolchain to auto-upgrade for dependencies requiring newer versions
ENV GOTOOLCHAIN=auto

# Copy go mod files first and download deps - this layer is cached unless go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (this layer changes frequently but deps are already cached)
COPY . .

# Build binaries
# CGO_ENABLED=0 creates static binaries compatible with alpine
RUN CGO_ENABLED=0 go build -o /app/bin/indexer ./cmd/indexer && \
    CGO_ENABLED=0 go build -o /app/bin/indexer-migrate ./cmd/indexer/migrate

# Runtime stage
FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /app/bin/indexer /app/indexer
COPY --from=builder /app/bin/indexer-migrate /app/indexer-migrate
COPY --from=builder /app/pkg/config/defaults /app/config/defaults
COPY scripts/setup/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENV DEFAULT_BINARY=/app/indexer
ENV ENV=docker

EXPOSE 8082

ENTRYPOINT ["/app/entrypoint.sh"]
