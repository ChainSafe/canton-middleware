package service

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// Web3API implements the web3_* JSON-RPC namespace.
type Web3API struct{}

func NewWeb3API() *Web3API {
	return &Web3API{}
}

func (*Web3API) ClientVersion() string {
	return "canton-middleware/1.0.0"
}

func (*Web3API) Sha3(input hexutil.Bytes) hexutil.Bytes {
	return crypto.Keccak256(input)
}
