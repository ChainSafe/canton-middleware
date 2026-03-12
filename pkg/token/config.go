package token

import (
	"github.com/ethereum/go-ethereum/common"
)

// ERC20Token contains ERC-20 token metadata
type ERC20Token struct {
	Name     string `yaml:"name" validate:"required"`
	Symbol   string `yaml:"symbol" validate:"required"`
	Decimals int    `yaml:"decimals" validate:"required,gt=0"`
}

// Config holds token metadata indexed by contract address.
type Config struct {
	SupportedTokens  map[common.Address]ERC20Token `yaml:"supported_tokens" validate:"required,min=1"`
	NativeBalanceWei string                        `yaml:"native_balance_wei" default:"1000000000000000000000"`
}

// NewConfig creates a token Config.
func NewConfig(nativeBalanceWei string) *Config {
	return &Config{
		NativeBalanceWei: nativeBalanceWei,
		SupportedTokens:  make(map[common.Address]ERC20Token),
	}
}

// AddToken registers a supported token contract.
func (c *Config) AddToken(address common.Address, token ERC20Token) {
	if c.SupportedTokens == nil {
		c.SupportedTokens = make(map[common.Address]ERC20Token)
	}
	c.SupportedTokens[address] = token
}
