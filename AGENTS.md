# Agent Instructions for Canton-Ethereum Bridge

## Build Commands

```bash
# Build the relayer
make build

# Run tests
make test

# Run with coverage
make test-coverage

# Format code
make fmt

# Lint code
make lint
```

## Development Workflow

1. Run database: `make db-up`
2. Apply schema: `make db-migrate`
3. Build: `make build`
4. Run: `make run` or `./bin/relayer -config config.yaml`

## Code Style

- Use Go standard formatting (gofmt)
- Follow Go naming conventions
- Use structured logging with zap
- Add Prometheus metrics for monitoring
- Database queries should use the Store abstraction

## Project Structure

- `cmd/relayer/` - Main application entry point
- `pkg/` - Public packages (canton, ethereum, relayer, db, config)
- `internal/` - Private packages (metrics, security)
- `contracts/` - Smart contracts (ethereum, canton)
- `deployments/` - Docker and k8s configs

## Testing

- Write unit tests for all packages
- Use table-driven tests where applicable
- Mock external dependencies (blockchain clients)
- Integration tests should use testnet connections

## Dependencies

- chi - HTTP router
- viper - Configuration management
- zap - Structured logging
- prometheus - Metrics
- lib/pq - PostgreSQL driver
- go-ethereum - Ethereum client
