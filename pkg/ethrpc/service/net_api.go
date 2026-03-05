package service

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// NetAPI implements the net_* JSON-RPC namespace.
type NetAPI struct {
	svc Service
}

func NewNetAPI(svc Service) *NetAPI {
	return &NetAPI{svc: svc}
}

func (api *NetAPI) Version() string {
	return fmt.Sprintf("%d", api.svc.ChainID(context.Background()))
}

func (*NetAPI) Listening() bool {
	return true
}

func (*NetAPI) PeerCount() hexutil.Uint {
	return hexutil.Uint(0)
}
