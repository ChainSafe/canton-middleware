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
RUN CGO_ENABLED=0 go build -o /app/bin/api-server ./cmd/api-server && \
    CGO_ENABLED=0 go build -o /app/bin/api-server-migrate ./cmd/api-server/migrate

# Runtime stage
FROM alpine:latest

WORKDIR /app
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /app/bin/api-server /app/api-server
COPY --from=builder /app/bin/api-server-migrate /app/api-server-migrate
COPY --from=builder /app/pkg/config/defaults /app/config/defaults
COPY scripts/setup/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENV DEFAULT_BINARY=/app/api-server
ENV ENV=docker

EXPOSE 8081

ENTRYPOINT ["/app/entrypoint.sh"]
