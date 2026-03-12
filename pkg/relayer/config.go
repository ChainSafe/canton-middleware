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
}
