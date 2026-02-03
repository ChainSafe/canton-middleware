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
-- Includes custodial Canton key fields for user-owned holdings
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    evm_address VARCHAR(42) UNIQUE NOT NULL,
    canton_party VARCHAR(255) NOT NULL,           -- User's Canton party ID
    fingerprint VARCHAR(128) NOT NULL,
    mapping_cid VARCHAR(255),
    prompt_balance DECIMAL(38,18) DEFAULT 0,      -- PROMPT (bridged) token balance
    demo_balance DECIMAL(38,18) DEFAULT 0,        -- DEMO (native) token balance
    balance_updated_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    -- Custodial Canton key fields
    canton_party_id VARCHAR(255),                 -- User's own Canton party (same as canton_party for new users)
    canton_private_key_encrypted TEXT,            -- AES-256-GCM encrypted Canton private key (base64)
    canton_key_created_at TIMESTAMP               -- When the Canton key was generated
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

-- Bridge events table: tracks processed bridge events for reconciliation
CREATE TABLE IF NOT EXISTS bridge_events (
    id SERIAL PRIMARY KEY,
    event_type VARCHAR(20) NOT NULL,  -- 'mint', 'burn'
    contract_id VARCHAR(255) UNIQUE NOT NULL,
    fingerprint VARCHAR(128),
    recipient_fingerprint VARCHAR(128),
    amount DECIMAL(38,18) NOT NULL,
    evm_tx_hash VARCHAR(66),
    evm_destination VARCHAR(42),
    token_symbol VARCHAR(20),
    canton_timestamp TIMESTAMP,
    processed_at TIMESTAMP DEFAULT NOW()
);

-- Reconciliation state table: tracks reconciliation progress
CREATE TABLE IF NOT EXISTS reconciliation_state (
    id INT PRIMARY KEY DEFAULT 1,
    last_processed_offset BIGINT DEFAULT 0,
    last_full_reconcile_at TIMESTAMP,
    events_processed INT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT single_reconcile_row CHECK (id = 1)
);

-- Initialize reconciliation state row
INSERT INTO reconciliation_state (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- EVM Transactions (MetaMask JSON-RPC compatibility)
-- =============================================================================

-- EVM Transactions table: stores synthetic tx receipts for Eth JSON-RPC facade
CREATE TABLE IF NOT EXISTS evm_transactions (
    tx_hash          BYTEA PRIMARY KEY,
    from_address     TEXT NOT NULL,
    to_address       TEXT NOT NULL,
    nonce            BIGINT NOT NULL,
    input            BYTEA NOT NULL,
    value_wei        TEXT NOT NULL DEFAULT '0',
    status           SMALLINT NOT NULL DEFAULT 1,
    block_number     BIGINT NOT NULL,
    block_hash       BYTEA NOT NULL,
    tx_index         INTEGER NOT NULL DEFAULT 0,
    gas_used         BIGINT NOT NULL DEFAULT 21000,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- EVM Metadata table: stores chain state like latest block number
CREATE TABLE IF NOT EXISTS evm_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Initialize latest block number
INSERT INTO evm_meta (key, value) VALUES ('latest_block_number', '0')
ON CONFLICT (key) DO NOTHING;

-- EVM Logs table: stores synthetic event logs for Eth JSON-RPC facade
CREATE TABLE IF NOT EXISTS evm_logs (
    tx_hash      BYTEA NOT NULL,
    log_index    INTEGER NOT NULL,
    address      BYTEA NOT NULL,
    topic0       BYTEA,
    topic1       BYTEA,
    topic2       BYTEA,
    topic3       BYTEA,
    data         BYTEA,
    block_number BIGINT NOT NULL,
    block_hash   BYTEA NOT NULL,
    tx_index     INTEGER NOT NULL DEFAULT 0,
    removed      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (tx_hash, log_index)
);

-- =============================================================================
-- Indexes
-- =============================================================================

CREATE INDEX IF NOT EXISTS idx_users_evm ON users(evm_address);
CREATE INDEX IF NOT EXISTS idx_users_fingerprint ON users(fingerprint);
CREATE INDEX IF NOT EXISTS idx_users_canton_party_id ON users(canton_party_id);
CREATE INDEX IF NOT EXISTS idx_bridge_events_fingerprint ON bridge_events(fingerprint);
CREATE INDEX IF NOT EXISTS idx_bridge_events_type ON bridge_events(event_type);
CREATE INDEX IF NOT EXISTS idx_bridge_events_evm_tx ON bridge_events(evm_tx_hash);
CREATE INDEX IF NOT EXISTS idx_evm_transactions_from ON evm_transactions(from_address);
CREATE INDEX IF NOT EXISTS idx_evm_transactions_to ON evm_transactions(to_address);
CREATE INDEX IF NOT EXISTS idx_evm_transactions_block ON evm_transactions(block_number);

