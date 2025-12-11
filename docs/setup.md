# Setup Guide

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- Foundry (forge, cast) for Ethereum tooling
- Access to Canton Network Partner Node (or local Docker setup)

## Local Development Setup

### 1. Clone and Install Dependencies

```bash
cd canton-middleware
make deps
```

### 2. Start Development Environment

The easiest way to run locally is with Docker Compose:

```bash
docker compose up -d
```

This starts:
- **Anvil**: Local Ethereum node
- **Canton**: Local Canton participant node  
- **PostgreSQL**: Database for relayer state

### 3. Configuration

```bash
cp config.example.yaml config.local.yaml
```

Edit `config.local.yaml` with your settings. For local development, the defaults should work.

### 4. Bootstrap the Bridge

After services are running, bootstrap the Canton bridge contracts:

```bash
# Get the dynamically allocated party ID
PARTY_ID=$(curl -s http://localhost:5013/v2/parties | jq -r '.partyDetails[0].party')

# Bootstrap the bridge
go run scripts/bootstrap-bridge.go -config config.local.yaml -issuer "$PARTY_ID"

# Register a test user
go run scripts/register-user.go -config config.local.yaml -party "$PARTY_ID"
```

### 5. Run the Relayer

```bash
make run
# Or with a specific config:
go run cmd/relayer/main.go -config config.local.yaml
```

## Automated Testing

For full end-to-end testing:

```bash
# Clean environment test
./scripts/test-bridge-local.sh --clean

# Skip Docker setup if services already running
./scripts/test-bridge-local.sh --skip-docker
```

## Building

```bash
make build
./bin/relayer -config config.local.yaml
```

## Database

Using Docker (recommended):
```bash
make db-up      # Start PostgreSQL container
make db-migrate # Run schema migrations
make db-down    # Stop and remove container
```

## Verifying Setup

### Health Check
```bash
curl http://localhost:8080/health
# Returns: OK
```

### Status Check
```bash
curl http://localhost:8080/api/v1/status | jq
```

### Metrics
```bash
curl http://localhost:9090/metrics
```

## Testing

```bash
make test           # Run unit tests
make test-coverage  # Run with coverage report
make test-contracts # Run Solidity tests
```

## Troubleshooting

### Database Connection Failed
- Verify PostgreSQL is running: `docker ps | grep postgres`
- Check credentials in config.yaml

### Cannot Connect to Canton
- Ensure Canton is healthy: `docker inspect --format='{{.State.Health.Status}}' canton`
- Verify synchronizer connection: `curl http://localhost:5013/v2/state/connected-synchronizers | jq`

### Cannot Connect to Ethereum (Anvil)
- Check Anvil is running: `docker ps | grep anvil`
- Test RPC: `cast block-number --rpc-url http://localhost:8545`

## Next Steps

See [BRIDGE_TESTING_GUIDE.md](BRIDGE_TESTING_GUIDE.md) for detailed testing instructions.
