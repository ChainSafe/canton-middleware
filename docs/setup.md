# Setup Guide

## Prerequisites

- Go 1.23 or higher
- PostgreSQL 15 or higher
- Docker and Docker Compose (optional)
- Access to Canton Network Partner Node
- Ethereum RPC endpoint (Infura, Alchemy, or own node)

## Local Development Setup

### 1. Clone and Install Dependencies

```bash
cd canton-middleware
make deps
```

### 2. Database Setup

Using Docker:
```bash
make db-up
make db-migrate
```

Or manually with PostgreSQL:
```bash
createdb -U postgres canton_bridge
psql -U postgres -d canton_bridge -f pkg/db/schema.sql
```

### 3. Configuration

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` with your settings:
- Database credentials
- Ethereum RPC URL and Infura key
- Canton Network RPC URL
- Bridge contract addresses
- Relayer private keys (or set via environment variables)

### 4. Run the Relayer

```bash
make run
```

Or build and run:
```bash
make build
./bin/relayer -config config.yaml
```

## Docker Setup

### 1. Configure Environment

Create `.env` file:
```bash
ETHEREUM_RELAYER_PRIVATE_KEY=your_ethereum_private_key
CANTON_RELAYER_PRIVATE_KEY=your_canton_private_key
```

### 2. Start Services

```bash
docker-compose up -d
```

This will start:
- PostgreSQL database
- Bridge relayer
- Prometheus (metrics)

### 3. Check Logs

```bash
docker-compose logs -f relayer
```

## Verifying Setup

### Health Check
```bash
curl http://localhost:8080/health
```

### Status Check
```bash
curl http://localhost:8080/api/v1/status
```

### Metrics
```bash
curl http://localhost:9090/metrics
```

## Database Migrations

Initial schema setup:
```bash
make db-migrate
```

## Testing

Run unit tests:
```bash
make test
```

Run with coverage:
```bash
make test-coverage
```

## Configuration Details

### Required Environment Variables

For security, set sensitive values via environment variables:

```bash
export ETHEREUM_RELAYER_PRIVATE_KEY="0x..."
export CANTON_RELAYER_PRIVATE_KEY="..."
export DATABASE_PASSWORD="..."
```

### Key Configuration Sections

**Ethereum**: Configure mainnet or testnet RPC, confirmation blocks, gas settings

**Canton**: Configure Canton Partner Node connection, bridge contract

**Bridge**: Set transfer limits, rate limiting, retry policies

**Monitoring**: Enable/disable metrics, configure ports

**Logging**: Set log level (debug, info, warn, error), format (json, console)

## Troubleshooting

### Database Connection Failed
- Verify PostgreSQL is running
- Check credentials in config.yaml
- Ensure database exists

### Cannot Connect to Ethereum
- Verify Infura API key
- Check network connectivity
- Validate RPC URL

### Cannot Connect to Canton
- Verify Canton Partner Node is accessible
- Check RPC endpoint configuration
- Ensure proper authentication

## Next Steps

1. Deploy smart contracts (see `docs/deployment.md`)
2. Configure bridge parameters
3. Monitor logs and metrics
4. Test with small transfers
5. Gradually increase limits
