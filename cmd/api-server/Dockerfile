# Build stage
# Using full golang image (not alpine) for toolchain auto-download support
FROM golang:1.23 AS builder

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
    GOTOOLCHAIN=auto go build -o /app/bin/api-server ./cmd/api-server

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /app/bin/api-server /app/api-server

EXPOSE 8080 9090

ENTRYPOINT ["/app/api-server"]
CMD ["-config", "/app/config.yaml"]
