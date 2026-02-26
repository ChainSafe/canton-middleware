package token

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/pkg/config"
)

// Type identifies a token for balance operations.
type Type string

const (
	Prompt Type = "PROMPT"
	Demo   Type = "DEMO"
)

// Config holds token metadata indexed by contract address.
type Config struct {
	SupportedTokens  map[common.Address]config.TokenConfig `yaml:"supported_tokens"`
	NativeBalanceWei string                                `yaml:"native_balance_wei"`
}

// NewConfig creates a token Config.
func NewConfig(nativeBalanceWei string) *Config {
	return &Config{
		NativeBalanceWei: nativeBalanceWei,
		SupportedTokens:  make(map[common.Address]config.TokenConfig),
	}
}

// AddToken registers a supported token contract.
func (c *Config) AddToken(address common.Address, token config.TokenConfig) {
	if c.SupportedTokens == nil {
		c.SupportedTokens = make(map[common.Address]config.TokenConfig)
	}
	c.SupportedTokens[address] = token
}

// getToken returns token metadata for a contract address.
func (c *Config) getToken(address common.Address) (config.TokenConfig, error) {
	if tkn, ok := c.SupportedTokens[address]; ok {
		return tkn, nil
	}
	return config.TokenConfig{}, fmt.Errorf("token not supported: %s", address.Hex())
}
