# Build stage
# Using full golang image (not alpine) for toolchain auto-download support
FROM golang:1.24 AS builder

WORKDIR /app

# Allow Go toolchain to auto-upgrade for dependencies requiring newer versions
ENV GOTOOLCHAIN=auto

# Copy go mod files first
COPY go.mod go.sum ./

# Copy all source
COPY . .

# Download deps and build in one layer to preserve toolchain
# GOTOOLCHAIN=auto downloads newer Go version if required by deps
RUN GOTOOLCHAIN=auto go mod download && \
    GOTOOLCHAIN=auto CGO_ENABLED=0 go build -ldflags="-w -s" -o /app/bin/api-server ./cmd/api-server && \
    GOTOOLCHAIN=auto CGO_ENABLED=0 go build -ldflags="-w -s" -o /app/bin/api-server-migrate ./cmd/api-server/migrate

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates wget

# Copy binaries and entrypoint from builder
COPY --from=builder /app/bin/api-server /app/api-server
COPY --from=builder /app/bin/api-server-migrate /app/api-server-migrate
COPY --from=builder /app/scripts/setup/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENV DEFAULT_BINARY=/app/api-server

EXPOSE 8081

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["-config", "/app/config.yaml"]
