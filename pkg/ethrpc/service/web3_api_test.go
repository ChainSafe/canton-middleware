package service_test

import (
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

func TestWeb3API_ClientVersion(t *testing.T) {
	api := service.NewWeb3API()
	assert.Equal(t, "canton-middleware/1.0.0", api.ClientVersion())
}

func TestWeb3API_Sha3(t *testing.T) {
	api := service.NewWeb3API()
	input := hexutil.Bytes("hello")
	got := api.Sha3(input)
	assert.Equal(t, hexutil.Bytes(crypto.Keccak256(input)), got)
}
