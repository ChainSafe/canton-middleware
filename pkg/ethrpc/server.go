package ethrpc

import (
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/apidb"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/service"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
)

// Server handles Ethereum JSON-RPC requests for MetaMask compatibility
type Server struct {
	cfg          *config.APIServerConfig
	db           *apidb.Store
	tokenService *service.TokenService
	logger       *zap.Logger

	chainID          *big.Int
	tokenAddress     common.Address // PROMPT token address
	demoTokenAddress common.Address // DEMO token address (native)
	erc20ABI         abi.ABI
	rpcServer        *rpc.Server
	startTime        time.Time // For simulating block progression
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

	parsedABI, err := abi.JSON(strings.NewReader(ethereum.ERC20ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ERC20 ABI: %w", err)
	}

	// Parse DEMO token address (optional)
	var demoTokenAddr common.Address
	if cfg.EthRPC.DemoTokenAddress != "" && common.IsHexAddress(cfg.EthRPC.DemoTokenAddress) {
		demoTokenAddr = common.HexToAddress(cfg.EthRPC.DemoTokenAddress)
	}

	s := &Server{
		cfg:              cfg,
		db:               db,
		tokenService:     tokenService,
		logger:           logger,
		chainID:          big.NewInt(int64(cfg.EthRPC.ChainID)),
		tokenAddress:     common.HexToAddress(cfg.EthRPC.TokenAddress),
		demoTokenAddress: demoTokenAddr,
		erc20ABI:         parsedABI,
		rpcServer:        rpc.NewServer(),
		startTime:        time.Now(),
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
		zap.String("token_address", cfg.EthRPC.TokenAddress),
		zap.String("demo_token_address", cfg.EthRPC.DemoTokenAddress))

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
