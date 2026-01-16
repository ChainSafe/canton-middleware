package ethrpc

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// Web3API implements the web3_* JSON-RPC namespace
type Web3API struct{}

// NewWeb3API creates a new Web3API instance
func NewWeb3API() *Web3API {
	return &Web3API{}
}

// ClientVersion returns the client version
func (api *Web3API) ClientVersion() string {
	return "canton-middleware/1.0.0"
}

// Sha3 returns the Keccak-256 hash of the input
func (api *Web3API) Sha3(input hexutil.Bytes) hexutil.Bytes {
	return crypto.Keccak256(input)
}
