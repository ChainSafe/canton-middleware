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
}
