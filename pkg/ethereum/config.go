// SPDX-License-Identifier: Apache-2.0

package ethereum

import "time"

// Config contains Ethereum client settings
type Config struct {
	RPCURL             string        `yaml:"rpc_url" validate:"required"`
	WSUrl              string        `yaml:"ws_url" default:""`
	ChainID            int64         `yaml:"chain_id" validate:"required,gt=0"`
	BridgeContract     string        `yaml:"bridge_contract" validate:"required"`
	TokenContract      string        `yaml:"token_contract" default:""`
	RelayerPrivateKey  string        `yaml:"relayer_private_key" validate:"required"`
	ConfirmationBlocks int           `yaml:"confirmation_blocks" default:"12"`
	GasLimit           uint64        `yaml:"gas_limit" validate:"required,gt=0"`
	MaxGasPrice        string        `yaml:"max_gas_price" default:""`
	PollingInterval    time.Duration `yaml:"polling_interval" validate:"required,gt=0"`
	StartBlock         int64         `yaml:"start_block" default:"0"`
	LookbackBlocks     int64         `yaml:"lookback_blocks" default:"1000"`
	MaxBlockRange      uint64        `yaml:"max_block_range" default:"100" validate:"required,gt=0"`
	// RPCTimeout bounds each individual JSON-RPC call (eth_getLogs, eth_blockNumber)
	// so a hung or slow provider fails that call — which is logged and retried on the
	// next poll tick — instead of stalling the poll loop. <=0 disables the timeout.
	RPCTimeout time.Duration `yaml:"rpc_timeout" default:"30s"`
}
