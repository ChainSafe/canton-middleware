package token

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// ERC20Token contains ERC-20 token metadata and Canton instrument mapping.
type ERC20Token struct {
	Name         string `yaml:"name" validate:"required"`
	Symbol       string `yaml:"symbol" validate:"required"`
	Decimals     int    `yaml:"decimals" validate:"gte=0,lte=18"`
	InstrumentID string `yaml:"instrument_id" validate:"required"`
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

// getToken returns the ERC20Token metadata for the given contract address,
// or an error if the address is not a supported token.
func (c *Config) getToken(address common.Address) (ERC20Token, error) {
	tkn, ok := c.SupportedTokens[address]
	if !ok {
		return ERC20Token{}, fmt.Errorf("token not supported: %s", address.Hex())
	}
	return tkn, nil
}
