package token

import (
	"fmt"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/ethereum/go-ethereum/common"
)

// Type represents a token type for balance operations.
// It's exactly the same as the token symbol for now.
type Type string

const (
	Prompt Type = "PROMPT" // PROMPT (bridged) token
	Demo   Type = "DEMO"   // DEMO (native) token
)

type Config struct {
	SupportedTokens  map[common.Address]config.TokenConfig `yaml:"supported_tokens"`
	NativeBalanceWei string                                `yaml:"native_balance_wei"`
}

func NewConfig(nativeBalanceWei string) *Config {
	return &Config{
		NativeBalanceWei: nativeBalanceWei,
		SupportedTokens:  make(map[common.Address]config.TokenConfig),
	}
}

func (c *Config) AddToken(address common.Address, token config.TokenConfig) {
	if c.SupportedTokens == nil {
		c.SupportedTokens = make(map[common.Address]config.TokenConfig)
	}
	c.SupportedTokens[address] = token
}

func (c *Config) getToken(address common.Address) (config.TokenConfig, error) {
	if tkn, ok := c.SupportedTokens[address]; ok {
		return tkn, nil
	}
	return config.TokenConfig{}, fmt.Errorf("token not supported: %s", address.Hex())
}
