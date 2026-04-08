//go:build e2e

package shim

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/chainsafe/canton-middleware/pkg/registry"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// APIServerShim implements stack.APIServer. REST calls go through httpClient;
// EVM calls go through the go-ethereum ethclient connected to /eth so that
// JSON-RPC compatibility of the facade is exercised with real client code.
type APIServerShim struct {
	httpClient
	evm *ethclient.Client
}

// NewAPIServer dials the api-server REST endpoint and its /eth JSON-RPC
// endpoint, returning a ready shim. The RPC connection is established with
// rpc.Dial (same pattern as pkg/ethrpc/service/eth_api_test.go).
func NewAPIServer(ctx context.Context, manifest *stack.ServiceManifest) (*APIServerShim, error) {
	rpcClient, err := rpc.DialContext(ctx, manifest.APIHTTP+"/eth")
	if err != nil {
		return nil, fmt.Errorf("dial api-server eth RPC: %w", err)
	}
	return &APIServerShim{
		httpClient: httpClient{
			endpoint: manifest.APIHTTP,
			client:   &http.Client{Timeout: 30 * time.Second},
		},
		evm: ethclient.NewClient(rpcClient),
	}, nil
}

func (a *APIServerShim) Endpoint() string          { return a.endpoint }
func (a *APIServerShim) RPC() *ethclient.Client    { return a.evm }

// Health returns nil when GET /health responds with 200.
func (a *APIServerShim) Health(ctx context.Context) error {
	return a.getOK(ctx, "/health")
}

// Register sends POST /register.
func (a *APIServerShim) Register(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error) {
	var resp user.RegisterResponse
	if err := a.post(ctx, "/register", "", "", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PrepareTopology sends POST /register/prepare-topology.
func (a *APIServerShim) PrepareTopology(ctx context.Context, req *user.RegisterRequest) (*user.PrepareTopologyResponse, error) {
	var resp user.PrepareTopologyResponse
	if err := a.post(ctx, "/register/prepare-topology", "", "", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RegisterExternal sends POST /register with key_mode=external.
func (a *APIServerShim) RegisterExternal(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error) {
	return a.Register(ctx, req)
}

// PrepareTransfer sends POST /api/v2/transfer/prepare with timed EIP-191 auth headers.
func (a *APIServerShim) PrepareTransfer(
	ctx context.Context,
	account *stack.Account,
	req *transfer.PrepareRequest,
) (*transfer.PrepareResponse, error) {
	msg := fmt.Sprintf("transfer:%d", time.Now().Unix())
	sig, err := signEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.PrepareResponse
	if err := a.post(ctx, "/api/v2/transfer/prepare", sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ExecuteTransfer sends POST /api/v2/transfer/execute with timed EIP-191 auth headers.
func (a *APIServerShim) ExecuteTransfer(
	ctx context.Context,
	account *stack.Account,
	req *transfer.ExecuteRequest,
) (*transfer.ExecuteResponse, error) {
	msg := fmt.Sprintf("execute:%d", time.Now().Unix())
	sig, err := signEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.ExecuteResponse
	if err := a.post(ctx, "/api/v2/transfer/execute", sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ERC20Balance returns the ERC-20 balance of ownerAddr for tokenAddr by
// calling balanceOf via the api-server's /eth JSON-RPC facade using the
// standard go-ethereum ethclient and contract binding.
func (a *APIServerShim) ERC20Balance(ctx context.Context, tokenAddr, ownerAddr common.Address) (*big.Int, error) {
	token, err := contracts.NewPromptToken(tokenAddr, a.evm)
	if err != nil {
		return nil, fmt.Errorf("bind erc20: %w", err)
	}
	bal, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, ownerAddr)
	if err != nil {
		return nil, fmt.Errorf("balanceOf via api-server RPC: %w", err)
	}
	return bal, nil
}

// TransferFactory sends POST /registry/transfer-instruction/v1/transfer-factory.
func (a *APIServerShim) TransferFactory(ctx context.Context) (*registry.TransferFactoryResponse, error) {
	var resp registry.TransferFactoryResponse
	if err := a.post(ctx, "/registry/transfer-instruction/v1/transfer-factory", "", "", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// signEIP191 produces a 0x-prefixed EIP-191 signature of message using the
// hex-encoded ECDSA private key. The recovery ID is set to 27 or 28 as
// required by the api-server's VerifyEIP191Signature.
func signEIP191(hexKey, message string) (string, error) {
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256Hash([]byte(prefix + message))
	sig, err := crypto.Sign(hash.Bytes(), key)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	sig[64] += 27 // normalise recovery ID to Ethereum convention (27/28)
	return "0x" + fmt.Sprintf("%x", sig), nil
}
