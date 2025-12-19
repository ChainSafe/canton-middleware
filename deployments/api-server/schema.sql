-- =============================================================================
-- ERC-20 API Server Database Schema
-- =============================================================================
-- This file contains the schema for the erc20_api database:
-- - users: EVM address to Canton party mappings
-- - whitelist: Allowed EVM addresses for registration
-- - token_metrics: Total supply and reconciliation metadata
--
-- NOTE: This script assumes it is run against the erc20_api database.
-- Database creation is handled by create-erc20-db.sh
-- =============================================================================

-- Users table: maps EVM addresses to Canton parties with cached balance
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    evm_address VARCHAR(42) UNIQUE NOT NULL,
    canton_party VARCHAR(255) NOT NULL,
    fingerprint VARCHAR(128) NOT NULL,
    mapping_cid VARCHAR(255),
    balance DECIMAL(38,18) DEFAULT 0,
    balance_updated_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Whitelist table: controls who can register
CREATE TABLE IF NOT EXISTS whitelist (
    evm_address VARCHAR(42) PRIMARY KEY,
    note VARCHAR(500),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Token metrics table: stores total supply and reconciliation info
CREATE TABLE IF NOT EXISTS token_metrics (
    id INT PRIMARY KEY DEFAULT 1,
    total_supply DECIMAL(38,18) DEFAULT 0,
    last_reconciled_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT single_row CHECK (id = 1)
);

-- Initialize token metrics row
INSERT INTO token_metrics (id, total_supply) VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;

-- Indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_users_evm ON users(evm_address);
CREATE INDEX IF NOT EXISTS idx_users_fingerprint ON users(fingerprint);

