# Build stage
# Using full golang image (not alpine) for toolchain auto-download support
FROM golang:1.24 AS builder

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
RUN CGO_ENABLED=0 go build -o /app/bin/relayer ./cmd/relayer && \
    CGO_ENABLED=0 go build -o /app/bin/relayer-migrate ./cmd/relayer/migrate

# Runtime stage
FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /app/bin/relayer /app/relayer
COPY --from=builder /app/bin/relayer-migrate /app/relayer-migrate
COPY --from=builder /app/pkg/config/defaults /app/config/defaults
COPY scripts/setup/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENV DEFAULT_BINARY=/app/relayer
ENV ENV=docker

EXPOSE 8080 9090

ENTRYPOINT ["/app/entrypoint.sh"]
