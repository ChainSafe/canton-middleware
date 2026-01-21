package ethrpc

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// NetAPI implements the net_* JSON-RPC namespace
type NetAPI struct {
	server *Server
}

// NewNetAPI creates a new NetAPI instance
func NewNetAPI(server *Server) *NetAPI {
	return &NetAPI{server: server}
}

// Version returns the network ID
func (api *NetAPI) Version() string {
	return fmt.Sprintf("%d", api.server.cfg.EthRPC.ChainID)
}

// Listening returns true (always listening)
func (api *NetAPI) Listening() bool {
	return true
}

// PeerCount returns the number of peers (always 0 for this facade)
func (api *NetAPI) PeerCount() hexutil.Uint {
	return hexutil.Uint(0)
}
