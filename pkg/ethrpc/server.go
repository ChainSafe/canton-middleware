package ethrpc

import (
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
)

// Standard ERC20 ABI for parsing calls
const erc20ABI = `[
	{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"symbol","outputs":[{"name":"","type":"string"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"type":"function"},
	{"constant":true,"inputs":[],"name":"totalSupply","outputs":[{"name":"","type":"uint256"}],"type":"function"},
	{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},
	{"constant":false,"inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"},
	{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"},
	{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"value","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"},
	{"constant":false,"inputs":[{"name":"from","type":"address"},{"name":"to","type":"address"},{"name":"value","type":"uint256"}],"name":"transferFrom","outputs":[{"name":"","type":"bool"}],"type":"function"}
]`

// Server handles Ethereum JSON-RPC requests for MetaMask compatibility
type Server struct {
	cfg          *config.APIServerConfig
	db           *apidb.Store
	tokenService *service.TokenService
	logger       *zap.Logger

	chainID      *big.Int
	tokenAddress common.Address
	erc20ABI     abi.ABI
	rpcServer    *rpc.Server
}

// NewServer creates a new Ethereum JSON-RPC server
func NewServer(
	cfg *config.APIServerConfig,
	db *apidb.Store,
	tokenService *service.TokenService,
	logger *zap.Logger,
) (*Server, error) {
	if cfg.EthRPC.TokenAddress == "" {
		return nil, fmt.Errorf("eth_rpc.token_address is required")
	}

	if !common.IsHexAddress(cfg.EthRPC.TokenAddress) {
		return nil, fmt.Errorf("invalid token address: %s", cfg.EthRPC.TokenAddress)
	}

	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ERC20 ABI: %w", err)
	}

	s := &Server{
		cfg:          cfg,
		db:           db,
		tokenService: tokenService,
		logger:       logger,
		chainID:      big.NewInt(int64(cfg.EthRPC.ChainID)),
		tokenAddress: common.HexToAddress(cfg.EthRPC.TokenAddress),
		erc20ABI:     parsedABI,
		rpcServer:    rpc.NewServer(),
	}

	ethAPI := NewEthAPI(s)
	if err := s.rpcServer.RegisterName("eth", ethAPI); err != nil {
		return nil, fmt.Errorf("failed to register eth API: %w", err)
	}

	netAPI := NewNetAPI(s)
	if err := s.rpcServer.RegisterName("net", netAPI); err != nil {
		return nil, fmt.Errorf("failed to register net API: %w", err)
	}

	web3API := NewWeb3API()
	if err := s.rpcServer.RegisterName("web3", web3API); err != nil {
		return nil, fmt.Errorf("failed to register web3 API: %w", err)
	}

	logger.Info("Ethereum JSON-RPC server initialized",
		zap.Uint64("chain_id", cfg.EthRPC.ChainID),
		zap.String("token_address", cfg.EthRPC.TokenAddress))

	return s, nil
}

// ServeHTTP handles HTTP requests
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.rpcServer.ServeHTTP(w, r)
}
