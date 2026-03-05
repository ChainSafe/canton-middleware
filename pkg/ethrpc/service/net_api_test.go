package service_test

import (
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/service/mocks"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNetAPI_Version(t *testing.T) {
	mockSvc := mocks.NewService(t)
	mockSvc.EXPECT().ChainID(mock.Anything).Return(hexutil.Uint64(42))
	api := service.NewNetAPI(mockSvc)
	assert.Equal(t, "42", api.Version())
}

func TestNetAPI_Listening(t *testing.T) {
	api := service.NewNetAPI(nil)
	assert.True(t, api.Listening())
}

func TestNetAPI_PeerCount(t *testing.T) {
	api := service.NewNetAPI(nil)
	assert.Equal(t, hexutil.Uint(0), api.PeerCount())
}
