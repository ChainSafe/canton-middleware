package token

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// Type identifies a token for balance operations.
type Type string

const (
	Prompt Type = "PROMPT"
	Demo   Type = "DEMO"
)

// ERC20Token contains ERC-20 token metadata
type ERC20Token struct {
	Name     string `yaml:"name"`
	Symbol   string `yaml:"symbol"`
	Decimals int    `yaml:"decimals"`
}

// Config holds token metadata indexed by contract address.
type Config struct {
	SupportedTokens  map[common.Address]ERC20Token `yaml:"supported_tokens"`
	NativeBalanceWei string                        `yaml:"native_balance_wei"`
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

// getToken returns token metadata for a contract address.
func (c *Config) getToken(address common.Address) (ERC20Token, error) {
	if tkn, ok := c.SupportedTokens[address]; ok {
		return tkn, nil
	}
	return ERC20Token{}, fmt.Errorf("token not supported: %s", address.Hex())
}
