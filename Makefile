.PHONY: build test clean run setup db-up db-down docker-build docker-run deploy-contracts

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

get_lint:
	if [ ! -f ./bin/golangci-lint ]; then \
		curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v2.9.0; \
	fi;

# Run linter
lint: get_lint
	./bin/golangci-lint run

# Format code
fmt:
	go fmt ./...

# Code generation
generate-protos:
	./scripts/generate-protos.sh

generate-eth-bindings:
	cd contracts/ethereum-wayfinder && \
	forge build && \
	mkdir -p ../../pkg/ethereum/contracts && \
	jq '.abi' out/CantonBridge.sol/CantonBridge.json > /tmp/CantonBridge.abi.json && \
	abigen --abi /tmp/CantonBridge.abi.json --pkg contracts --type CantonBridge --out ../../pkg/ethereum/contracts/bridge.go && \
	jq '.abi' out/PromptToken.sol/PromptToken.json > /tmp/PromptToken.abi.json && \
	abigen --abi /tmp/PromptToken.abi.json --pkg contracts --type PromptToken --out ../../pkg/ethereum/contracts/prompt_token.go

generate: generate-protos generate-eth-bindings

# Ethereum contract operations
ETH_RPC_URL ?= http://localhost:8545
PRIVATE_KEY ?= 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
RELAYER_ADDRESS ?= 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
USER ?= 0x70997970C51812dc3A010C7d01b50e0d17dc79C8

deploy-contracts:
	cd contracts/ethereum-wayfinder && \
	PRIVATE_KEY=$(PRIVATE_KEY) RELAYER_ADDRESS=$(RELAYER_ADDRESS) USER=$(USER) \
	forge script script/Deploy.s.sol:DeployScript --rpc-url $(ETH_RPC_URL) --broadcast

deploy-contracts-dry-run:
	cd contracts/ethereum-wayfinder && \
	PRIVATE_KEY=$(PRIVATE_KEY) RELAYER_ADDRESS=$(RELAYER_ADDRESS) USER=$(USER) \
	forge script script/Deploy.s.sol:DeployScript --rpc-url $(ETH_RPC_URL)

test-contracts:
	cd contracts/ethereum-wayfinder && forge test -vvv

# Database setup
db-up:
	docker run --name canton-bridge-db -e POSTGRES_PASSWORD=changeme -e POSTGRES_USER=bridge -e POSTGRES_DB=canton_bridge -p 5432:5432 -d postgres:15

db-down:
	docker stop canton-bridge-db
	docker rm canton-bridge-db

db-migrate:
	@echo "Running relayer database migrations..."
	go run ./cmd/relayer/migrate/main.go -config config.yaml up
	@echo "Running API server database migrations..."
	go run ./cmd/api-server/migrate/main.go -config config.api-server.yaml up

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
