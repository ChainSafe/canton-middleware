// SPDX-License-Identifier: Apache-2.0

package relayer

import "time"

// Config contains bridge operation settings
type Config struct {
	MaxTransferAmount  string        `yaml:"max_transfer_amount" validate:"required"`
	MinTransferAmount  string        `yaml:"min_transfer_amount" validate:"required"`
	RateLimitPerHour   int           `yaml:"rate_limit_per_hour" validate:"required,gt=0"`
	MaxRetries         int           `yaml:"max_retries" default:"5"`
	RetryDelay         time.Duration `yaml:"retry_delay" default:"60s"`
	ProcessingInterval time.Duration `yaml:"processing_interval" default:"30s"`

	// Canton stream settings
	CantonChainID    string `yaml:"canton_chain_id" validate:"required"`
	CantonStartBlock uint64 `yaml:"canton_start_block" default:"0"`
	CantonLookback   int64  `yaml:"canton_lookback_blocks" default:"1000"`

	// ETH stream settings
	EthChainID        string `yaml:"eth_chain_id" validate:"required"`
	EthStartBlock     uint64 `yaml:"eth_start_block" default:"0"`
	EthLookbackBlocks int64  `yaml:"eth_lookback_blocks" default:"1000"`
	EthTokenContract  string `yaml:"eth_token_contract" validate:"required"`

	// Tokens maps token symbol to its bridged-token config. Tokens listed
	// here are driven by TokenBridge adapters; the legacy single-token
	// pipeline is configured by the Eth*/Canton* fields above.
	Tokens map[string]TokenConfig `yaml:"tokens" validate:"omitempty,dive"`
}

// TokenConfig declares one bridged token and the mechanism that moves it.
type TokenConfig struct {
	// Mechanism selects the TokenBridge adapter (e.g. "xreserve").
	Mechanism string `yaml:"mechanism" validate:"required"`
	// EVMAddress is the token's ERC-20 contract address on the EVM chain.
	EVMAddress string `yaml:"evm_address" validate:"required"`
	// Decimals is the token's on-chain decimal precision.
	Decimals int `yaml:"decimals" validate:"required,gt=0"`

	// XReserve carries the Circle xReserve settings; required when
	// mechanism is "xreserve" (enforced by the adapter constructor).
	XReserve *XReserveConfig `yaml:"xreserve" default:"-"`
}

// XReserveConfig configures tracking for a token bridged by Circle xReserve.
type XReserveConfig struct {
	// AttestationURL is the base URL of the attestation API (Circle's in
	// production, the devstack stub locally).
	AttestationURL string `yaml:"attestation_url" validate:"required"`
	// InstrumentAdmin is the Canton party administering the instrument
	// (e.g. Circle's decentralized-usdc-interchain-rep party).
	InstrumentAdmin string `yaml:"instrument_admin" validate:"required"`
	// InstrumentID is the Canton token-standard instrument id (e.g. "USDCx").
	InstrumentID string `yaml:"instrument_id" validate:"required"`
	// AttestationPollInterval paces attestation polling (default 60s).
	AttestationPollInterval time.Duration `yaml:"attestation_poll_interval"`
	// MintPollInterval paces post-attestation mint detection (default 15s).
	MintPollInterval time.Duration `yaml:"mint_poll_interval"`
}
