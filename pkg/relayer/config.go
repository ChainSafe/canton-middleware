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
}
