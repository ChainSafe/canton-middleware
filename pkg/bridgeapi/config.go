// SPDX-License-Identifier: Apache-2.0

package bridgeapi

import "fmt"

// Config configures the api-server bridge endpoints.
type Config struct {
	// RelayerURL is the relayer's HTTP base URL for transfer registration
	// and status lookups.
	RelayerURL string `yaml:"relayer_url" validate:"required"`
	// EthRPCURL optionally points at an EVM JSON-RPC node used to check
	// ERC-20 allowances. When unset, quotes always include the approve step
	// and the wallet may skip it client-side.
	EthRPCURL string `yaml:"eth_rpc_url"`
	// ChainID is the EVM chain the quoted transactions target.
	ChainID int64 `yaml:"chain_id" validate:"required"`
	// Tokens maps token symbol to its bridge settings.
	Tokens map[string]TokenConfig `yaml:"tokens" validate:"required,min=1,dive"`
}

// TokenConfig declares one bridgeable token on the api-server side.
type TokenConfig struct {
	// Mechanism selects the quoting strategy and the relayer bridge key.
	Mechanism string `yaml:"mechanism" validate:"required"`
	// EVMAddress is the token's ERC-20 contract on the EVM chain.
	EVMAddress string `yaml:"evm_address" validate:"required"`
	// Decimals is the token's on-chain decimal precision.
	Decimals int `yaml:"decimals" validate:"gte=0,lte=18"`
	// BridgeFee is a display-only indicative fee in token units.
	BridgeFee string `yaml:"bridge_fee"`
	// EstimatedSeconds is the indicative end-to-end deposit latency.
	EstimatedSeconds int `yaml:"estimated_seconds"`

	// XReserve is required when mechanism is "xreserve".
	XReserve *XReserveTokenConfig `yaml:"xreserve" default:"-"`
}

// XReserveTokenConfig carries the Circle xReserve bridge parameters.
type XReserveTokenConfig struct {
	// Contract is the xReserve contract address on the EVM chain.
	Contract string `yaml:"contract" validate:"required"`
	// RemoteDomain is the xReserve domain of the destination chain
	// (Canton mainnet: 10001).
	RemoteDomain uint32 `yaml:"remote_domain" validate:"required"`
	// MaxFee is the depositToRemote maxFee argument in token base units.
	MaxFee string `yaml:"max_fee"`
	// InstrumentAdmin is the Canton party administering the instrument
	// (selects the burn registry via the token client's external_tokens map).
	InstrumentAdmin string `yaml:"instrument_admin" validate:"required"`
	// InstrumentID is the Canton token-standard instrument id (e.g. "USDCx").
	InstrumentID string `yaml:"instrument_id" validate:"required"`
	// WithdrawDestinationDomain is the xReserve domain burns release to
	// (0 = Ethereum).
	WithdrawDestinationDomain int64 `yaml:"withdraw_destination_domain"`
}

// Validate performs the cross-field checks the tag validator cannot express.
func (c *Config) Validate() error {
	for symbol, tc := range c.Tokens {
		if tc.Mechanism == MechanismXReserve && tc.XReserve == nil {
			return fmt.Errorf("bridge token %s: xreserve config block is required", symbol)
		}
	}
	return nil
}
