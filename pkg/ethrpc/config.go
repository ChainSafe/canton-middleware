package ethrpc

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Config contains Ethereum JSON-RPC facade settings for MetaMask compatibility
type Config struct {
	Enabled          bool           `yaml:"enabled"`
	ChainID          uint64         `yaml:"chain_id"`
	TokenAddress     common.Address `yaml:"token_address"`
	DemoTokenAddress common.Address `yaml:"demo_token_address"`
	GasPriceWei      string         `yaml:"gas_price_wei"`
	GasLimit         uint64         `yaml:"gas_limit"`
	NativeBalanceWei string         `yaml:"native_balance_wei"`
	RequestTimeout   time.Duration  `yaml:"request_timeout"`
}
