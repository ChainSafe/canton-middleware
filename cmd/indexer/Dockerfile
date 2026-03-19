# Build stage
# Using full golang image (not alpine) for toolchain auto-download support
FROM golang:1.24 AS builder

WORKDIR /app

# Allow Go toolchain to auto-upgrade for dependencies requiring newer versions
ENV GOTOOLCHAIN=auto

# Copy go mod files first
COPY go.mod go.sum ./

# Download deps and build in one layer to preserve toolchain
RUN GOTOOLCHAIN=auto go mod download && \
    GOTOOLCHAIN=auto CGO_ENABLED=0 go build -ldflags="-w -s" -o /app/bin/indexer ./cmd/indexer && \
    GOTOOLCHAIN=auto CGO_ENABLED=0 go build -ldflags="-w -s" -o /app/bin/indexer-migrate ./cmd/indexer/migrate

# Copy all source
COPY . .

# Runtime stage
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/bin/indexer /app/indexer
COPY --from=builder /app/bin/indexer-migrate /app/indexer-migrate

EXPOSE 8082

ENTRYPOINT ["/app/indexer"]
CMD ["-config", "/app/config.yaml"]
