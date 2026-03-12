package ethereum

import "time"

// Config contains Ethereum client settings
type Config struct {
	RPCURL             string        `yaml:"rpc_url"`
	WSUrl              string        `yaml:"ws_url"`
	ChainID            int64         `yaml:"chain_id"`
	BridgeContract     string        `yaml:"bridge_contract"`
	TokenContract      string        `yaml:"token_contract"`
	RelayerPrivateKey  string        `yaml:"relayer_private_key"`
	ConfirmationBlocks int           `yaml:"confirmation_blocks"`
	GasLimit           uint64        `yaml:"gas_limit"`
	MaxGasPrice        string        `yaml:"max_gas_price"`
	PollingInterval    time.Duration `yaml:"polling_interval"`
	StartBlock         int64         `yaml:"start_block"`
	LookbackBlocks     int64         `yaml:"lookback_blocks"`
}
