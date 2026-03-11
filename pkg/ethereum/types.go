package ethereum

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Config is imported from config package to avoid duplication
// See: pkg/config/config.go for EthereumConfig definition

// DepositEvent represents a deposit to Canton event from Ethereum
type DepositEvent struct {
	Token           common.Address
	Sender          common.Address
	CantonRecipient [32]byte
	Amount          *big.Int
	Nonce           *big.Int
	BlockNumber     uint64
	TxHash          common.Hash
	LogIndex        uint
}

// WithdrawalEvent represents a withdrawal from Canton event on Ethereum
type WithdrawalEvent struct {
	Token        common.Address
	Recipient    common.Address
	Amount       *big.Int
	Nonce        *big.Int
	CantonTxHash [32]byte
	BlockNumber  uint64
	TxHash       common.Hash
}

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
