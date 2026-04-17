.PHONY: build test clean run setup db-up db-down docker-build docker-run deploy-contracts install-mockery check-mockery generate-mocks devstack-up devstack-down test-e2e test-e2e-api test-e2e-bridge test-e2e-indexer lint lint-e2e test-coverage test-coverage-check

GREEN := \033[0;32m
RED := \033[0;31m
RESET := \033[0m

MOCKERY_VERSION ?= v2.53.6
COVERAGE_THRESHOLD ?= 10

# Build the relayer binary
build:
	go build -o bin/relayer ./cmd/relayer

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1

# Run tests with coverage and enforce minimum threshold
test-coverage-check: test-coverage
	@total=$$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $$3}' | tr -d '%'); \
	echo "Coverage: $${total}% (threshold: $(COVERAGE_THRESHOLD)%)"; \
	if [ $$(awk "BEGIN {print ($${total} < $(COVERAGE_THRESHOLD))}") -eq 1 ]; then \
		echo "$(RED)FAIL: Coverage $${total}% is below $(COVERAGE_THRESHOLD)% threshold$(RESET)"; \
		exit 1; \
	else \
		echo "$(GREEN)PASS: Coverage $${total}% meets $(COVERAGE_THRESHOLD)% threshold$(RESET)"; \
	fi

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

install-mockery:
	go install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

get_lint:
	if [ ! -f ./bin/golangci-lint ]; then \
		curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v2.9.0; \
	fi;

# Run linter (main code + E2E tests)
lint: get_lint
	./bin/golangci-lint run
	./bin/golangci-lint run --build-tags e2e ./tests/e2e/...

# Run linter for E2E tests only
lint-e2e: get_lint
	./bin/golangci-lint run --build-tags e2e ./tests/e2e/...

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

CANTON_MASTER_KEY := $(or $(CANTON_MASTER_KEY),$(shell openssl rand -base64 32))
export CANTON_MASTER_KEY

devstack-up:
	go run ./tests/e2e/cmd/devstack up

devstack-down:
	go run ./tests/e2e/cmd/devstack down

test-e2e-api: devstack-up
	go test -v -tags e2e -timeout 10m -parallel 4 ./tests/e2e/tests/api/...

test-e2e-bridge: devstack-up
	go test -v -tags e2e -timeout 15m -parallel 4 ./tests/e2e/tests/bridge/...

test-e2e-indexer: devstack-up
	go test -v -tags e2e -timeout 20m -parallel 4 ./tests/e2e/tests/indexer/...

test-e2e: devstack-up test-e2e-api test-e2e-bridge test-e2e-indexer
