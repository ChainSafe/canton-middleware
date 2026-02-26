package service

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// NetAPI implements the net_* JSON-RPC namespace.
type NetAPI struct {
	svc Service
}

func (api *NetAPI) Version() string {
	return fmt.Sprintf("%d", api.svc.ChainID())
}

func (api *NetAPI) Listening() bool {
	return true
}

func (api *NetAPI) PeerCount() hexutil.Uint {
	return hexutil.Uint(0)
}
