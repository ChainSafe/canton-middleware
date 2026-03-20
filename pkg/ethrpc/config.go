package ethrpc

import (
	"time"
)

// Config contains Ethereum JSON-RPC facade settings for MetaMask compatibility
type Config struct {
	Enabled          bool          `yaml:"enabled" default:"false"`
	ChainID          uint64        `yaml:"chain_id" validate:"required_if=Enabled true"`
	GasPriceWei      string        `yaml:"gas_price_wei" default:"1000000000"`
	GasLimit         uint64        `yaml:"gas_limit" default:"21000"`
	NativeBalanceWei string        `yaml:"native_balance_wei" default:"1000000000000000000000"`
	RequestTimeout   time.Duration `yaml:"request_timeout"  default:"30s"`
}
