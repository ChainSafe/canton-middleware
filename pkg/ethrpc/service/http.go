package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// HTTP handles Ethereum JSON-RPC requests over HTTP.
type HTTP struct {
	rpcServer *rpc.Server
}

// RegisterRoutes registers the Ethereum JSON-RPC endpoint on the given chi router.
func RegisterRoutes(r chi.Router, svc Service, rpcRequestTimeout time.Duration, logger *zap.Logger) {
	rpcSrv := rpc.NewServer()

	if err := rpcSrv.RegisterName("eth", &EthAPI{svc: svc}); err != nil {
		panic(fmt.Sprintf("ethrpc: register eth API: %v", err))
	}
	if err := rpcSrv.RegisterName("net", NewNetAPI(svc)); err != nil {
		panic(fmt.Sprintf("ethrpc: register net API: %v", err))
	}
	if err := rpcSrv.RegisterName("web3", NewWeb3API()); err != nil {
		panic(fmt.Sprintf("ethrpc: register web3 API: %v", err))
	}

	h := &HTTP{rpcServer: rpcSrv}

	r.With(middleware.Timeout(rpcRequestTimeout)).Handle("/eth", http.HandlerFunc(h.handle))

	logger.Info("Ethereum JSON-RPC endpoint enabled", zap.String("path", "/eth"))
}

// handle processes an Ethereum JSON-RPC request.
// Unlike user service handlers, this does not return an error: the go-ethereum rpc.Server
// always responds HTTP 200 and encodes any failures as JSON-RPC error objects.
func (h *HTTP) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	h.rpcServer.ServeHTTP(w, r)
}
