#!/bin/bash
# =============================================================================
# Service management for Canton-Ethereum bridge tests
# =============================================================================

# Wait for HTTP endpoint to be ready
wait_for_endpoint() {
    local url=$1
    local name=$2
    local max_attempts=60
    local interval=3

    print_step "Waiting for $name..."

    for ((i=1; i<=max_attempts; i++)); do
        if curl -s -f "$url" > /dev/null 2>&1; then
            print_success "$name is ready"
            return 0
        fi
        sleep $interval
    done

    print_error "$name not ready after $max_attempts attempts"
}

# Wait for Anvil (JSON-RPC)
wait_for_anvil() {
    local max_attempts=60
    local interval=3

    print_step "Waiting for Anvil..."

    for ((i=1; i<=max_attempts; i++)); do
        if cast block-number --rpc-url "$ANVIL_URL" > /dev/null 2>&1; then
            print_success "Anvil is ready"
            return 0
        fi
        sleep $interval
    done

    print_error "Anvil not ready after $max_attempts attempts"
}

# Wait for all services
wait_for_all_services() {
    print_header "Waiting for Services"
    wait_for_anvil
    wait_for_endpoint "$API_SERVER_URL/health" "API Server"
    wait_for_endpoint "$RELAYER_URL/health" "Relayer"
    print_success "All services are healthy"
}

# Start Docker services
start_docker_services() {
    local verbose=$1

    print_header "Starting Docker Services"
    print_step "Running: docker compose up -d --build"

    if [ "$verbose" = "true" ]; then
        docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml up -d --build
    else
        docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml up -d --build > /dev/null 2>&1
    fi

    print_success "Docker services started"
}

# Stop Docker services
stop_docker_services() {
    print_header "Cleanup"
    print_step "Stopping Docker services..."
    docker compose -f docker-compose.yaml -f docker-compose.local-test.yaml down -v > /dev/null 2>&1
    print_success "Docker services stopped"
}
