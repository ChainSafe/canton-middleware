.PHONY: build test clean run setup db-up db-down docker-build docker-run

# Build the relayer binary
build:
	go build -o bin/relayer ./cmd/relayer

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Run the relayer
run:
	go run ./cmd/relayer -config config.yaml

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Code generation
generate-protos:
	./scripts/generate-protos.sh

generate-eth-bindings:
	cd contracts/ethereum && \
	forge build && \
	mkdir -p ../../pkg/ethereum/contracts && \
	abigen --abi out/CantonBridge.sol/CantonBridge.json --pkg contracts --type CantonBridge --out ../../pkg/ethereum/contracts/bridge.go && \
	abigen --abi out/WrappedCantonToken.sol/WrappedCantonToken.json --pkg contracts --type WrappedToken --out ../../pkg/ethereum/contracts/wrapped_token.go

generate: generate-protos generate-eth-bindings

# Database setup
db-up:
	docker run --name canton-bridge-db -e POSTGRES_PASSWORD=changeme -e POSTGRES_USER=bridge -e POSTGRES_DB=canton_bridge -p 5432:5432 -d postgres:15

db-down:
	docker stop canton-bridge-db
	docker rm canton-bridge-db

db-migrate:
	psql -h localhost -U bridge -d canton_bridge -f pkg/db/schema.sql

# Docker build
docker-build:
	docker build -t canton-bridge-relayer:latest .

docker-run:
	docker-compose up -d

# Development setup
setup: deps db-up
	@echo "Waiting for database to be ready..."
	@sleep 3
	$(MAKE) db-migrate
	cp config.example.yaml config.yaml
	@echo "Setup complete! Edit config.yaml and run 'make run'"
