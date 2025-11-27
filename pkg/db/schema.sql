-- Enable UUID extension if needed, though we use string IDs here
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Transfers table
CREATE TABLE IF NOT EXISTS transfers (
    id VARCHAR(255) PRIMARY KEY,
    direction VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    source_chain VARCHAR(100) NOT NULL,
    destination_chain VARCHAR(100) NOT NULL,
    source_tx_hash VARCHAR(255) NOT NULL,
    destination_tx_hash VARCHAR(255),
    token_address VARCHAR(255) NOT NULL,
    amount VARCHAR(255) NOT NULL,
    sender VARCHAR(255) NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    nonce BIGINT NOT NULL,
    source_block_number BIGINT NOT NULL,
    confirmation_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0
);

-- Indexes for transfers
CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_direction ON transfers(direction);
CREATE INDEX IF NOT EXISTS idx_transfers_source_tx_hash ON transfers(source_tx_hash);

-- Chain State table
CREATE TABLE IF NOT EXISTS chain_state (
    chain_id VARCHAR(100) PRIMARY KEY,
    last_block BIGINT NOT NULL,
    last_block_hash VARCHAR(255) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Nonce State table
CREATE TABLE IF NOT EXISTS nonce_state (
    chain_id VARCHAR(100) NOT NULL,
    address VARCHAR(255) NOT NULL,
    nonce BIGINT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chain_id, address)
);

-- Bridge Balances table
CREATE TABLE IF NOT EXISTS bridge_balances (
    chain_id VARCHAR(100) NOT NULL,
    token_address VARCHAR(255) NOT NULL,
    balance VARCHAR(255) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chain_id, token_address)
);
