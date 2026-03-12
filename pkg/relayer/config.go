package relayer

import "time"

// Config contains bridge operation settings
type Config struct {
	MaxTransferAmount  string        `yaml:"max_transfer_amount"`
	MinTransferAmount  string        `yaml:"min_transfer_amount"`
	RateLimitPerHour   int           `yaml:"rate_limit_per_hour"`
	MaxRetries         int           `yaml:"max_retries"`
	RetryDelay         time.Duration `yaml:"retry_delay"`
	ProcessingInterval time.Duration `yaml:"processing_interval"`

	// Canton stream settings
	CantonChainID    string `yaml:"canton_chain_id"`
	CantonStartBlock uint64 `yaml:"canton_start_block"`
	CantonLookback   int64  `yaml:"canton_lookback_blocks"`

	// ETH stream settings
	EthChainID        string `yaml:"eth_chain_id"`
	EthStartBlock     uint64 `yaml:"eth_start_block"`
	EthLookbackBlocks int64  `yaml:"eth_lookback_blocks"`
	EthTokenContract  string `yaml:"eth_token_contract"`
}
