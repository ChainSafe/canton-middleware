//go:build e2e

package shim

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	siwe "github.com/spruceid/siwe-go"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/chainsafe/canton-middleware/pkg/ethereum/contracts"
	"github.com/chainsafe/canton-middleware/pkg/registry"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/pkg/user/whitelist"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/util"
)

// SIWE login parameters. They MUST match the api-server's auth config
// (pkg/config/defaults/config.api-server.docker.yaml) or /auth/login rejects the
// signed message.
const (
	siweDomain  = "localhost"
	siweURI     = "http://localhost"
	siweChainID = 31337
)

// defaultAdminAPIKey is the admin bearer token used when ADMIN_API_KEY is not
// set in the environment. It MUST match the default in the api-server's compose
// environment (docker-compose.yaml: ADMIN_API_KEY: "${ADMIN_API_KEY:-...}") so
// the token the test sends matches the one the server validates.
const defaultAdminAPIKey = "local-admin-key"

// adminAPIKey returns the admin bearer token, preferring ADMIN_API_KEY from the
// environment so the test and the api-server container stay in sync.
func adminAPIKey() string {
	if k := os.Getenv("ADMIN_API_KEY"); k != "" {
		return k
	}
	return defaultAdminAPIKey
}

var _ stack.APIServer = (*APIServerShim)(nil)

// APIServerShim implements stack.APIServer. REST calls go through httpClient;
// EVM calls go through the go-ethereum ethclient connected to /eth so that
// JSON-RPC compatibility of the facade is exercised with real client code.
type APIServerShim struct {
	httpClient
	evm *ethclient.Client

	// tokens caches one SIWE-issued JWT per account address so the read endpoints
	// (which require a bearer token when auth is enabled) reuse a single login.
	mu     sync.Mutex
	tokens map[common.Address]string
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
		evm:    ethclient.NewClient(rpcClient),
		tokens: make(map[common.Address]string),
	}, nil
}

// Login performs the SIWE (EIP-4361) login flow for account and returns a JWT:
// fetch a nonce, build and EIP-191-sign the message, then exchange it at
// /auth/login. The account must already be registered. The token is cached, so
// the read endpoints authenticate transparently after the first call.
func (a *APIServerShim) Login(ctx context.Context, account *stack.Account) (string, error) {
	var nr auth.NonceResponse
	q := url.Values{"address": []string{account.Address.Hex()}}
	if err := a.get(ctx, "/auth/nonce", q, &nr); err != nil {
		return "", fmt.Errorf("fetch nonce: %w", err)
	}

	// Build the message with the same library the server parses with, so the text
	// is a guaranteed-parseable EIP-4361 message.
	msg, err := siwe.InitMessage(siweDomain, account.Address.Hex(), siweURI, nr.Nonce, map[string]any{
		"chainId":  siweChainID,
		"issuedAt": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", fmt.Errorf("build SIWE message: %w", err)
	}
	raw := msg.String()

	sig, err := util.SignEIP191(account.PrivateKey, raw)
	if err != nil {
		return "", fmt.Errorf("sign SIWE message: %w", err)
	}

	var lr auth.LoginResponse
	if err := a.post(ctx, "/auth/login", "", "", auth.LoginRequest{Message: raw, Signature: sig}, &lr); err != nil {
		return "", fmt.Errorf("login: %w", err)
	}

	a.mu.Lock()
	a.tokens[account.Address] = lr.Token
	a.mu.Unlock()
	return lr.Token, nil
}

// ensureToken returns a cached JWT for account, logging in on first use.
func (a *APIServerShim) ensureToken(ctx context.Context, account *stack.Account) (string, error) {
	a.mu.Lock()
	tok := a.tokens[account.Address]
	a.mu.Unlock()
	if tok != "" {
		return tok, nil
	}
	return a.Login(ctx, account)
}

func (a *APIServerShim) Endpoint() string       { return a.endpoint }
func (a *APIServerShim) RPC() *ethclient.Client { return a.evm }
func (a *APIServerShim) Close()                 { a.evm.Close() }

// Health returns nil when GET /health responds with 200.
func (a *APIServerShim) Health(ctx context.Context) error {
	return a.getOK(ctx, "/health")
}

// WhitelistAddress whitelists evmAddress via the admin API
// (POST /admin/whitelist) using the configured admin bearer token. This is how
// tests grant an address permission to register, exercising the real admin
// endpoint rather than writing to the database directly.
func (a *APIServerShim) WhitelistAddress(ctx context.Context, evmAddress string) error {
	body := map[string]string{"evm_address": evmAddress, "note": "e2e-test"}
	return a.postBearer(ctx, "/admin/whitelist", adminAPIKey(), body, nil)
}

// RemoveWhitelistAddress removes evmAddress from the whitelist via the admin API
// (DELETE /admin/whitelist/{address}). Returns a *HTTPError with Code 404 when
// the address was not whitelisted.
func (a *APIServerShim) RemoveWhitelistAddress(ctx context.Context, evmAddress string) error {
	return a.deleteBearer(ctx, "/admin/whitelist/"+evmAddress, adminAPIKey(), nil)
}

// ListWhitelist returns one cursor-delimited page of whitelist entries via the
// admin API (GET /admin/whitelist?cursor=&limit=). An empty cursor starts from
// the beginning; limit <= 0 lets the server apply its default.
func (a *APIServerShim) ListWhitelist(ctx context.Context, cursor string, limit int) (*whitelist.Page, error) {
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/admin/whitelist"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var page whitelist.Page
	if err := a.getBearer(ctx, path, adminAPIKey(), &page); err != nil {
		return nil, err
	}
	return &page, nil
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
// req.KeyMode must be user.KeyModeExternal; req must include RegistrationToken
// and TopologySignature obtained from PrepareTopology.
func (a *APIServerShim) RegisterExternal(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error) {
	if req.KeyMode != user.KeyModeExternal {
		return nil, fmt.Errorf("RegisterExternal: req.KeyMode must be %q, got %q", user.KeyModeExternal, req.KeyMode)
	}
	return a.Register(ctx, req)
}

// PrepareTransfer sends POST /api/v2/transfer/prepare with timed EIP-191 auth headers.
func (a *APIServerShim) PrepareTransfer(
	ctx context.Context,
	account *stack.Account,
	req *transfer.PrepareRequest,
) (*transfer.PrepareResponse, error) {
	msg := fmt.Sprintf("transfer:%d", time.Now().Unix())
	sig, err := util.SignEIP191(account.PrivateKey, msg)
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
	sig, err := util.SignEIP191(account.PrivateKey, msg)
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

// ListIncomingTransfers sends GET /api/v2/transfer/incoming as account. The caller
// is taken from account's SIWE-issued bearer token, so it returns only account's
// pending offers.
func (a *APIServerShim) ListIncomingTransfers(
	ctx context.Context,
	account *stack.Account,
) (*transfer.IncomingTransfersList, error) {
	token, err := a.ensureToken(ctx, account)
	if err != nil {
		return nil, err
	}
	var resp transfer.IncomingTransfersList
	if err := a.getBearer(ctx, "/api/v2/transfer/incoming", token, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SendCustodial sends POST /api/v2/transfer/custodial with timed EIP-191 auth
// headers. The middleware holds the custodial user's Canton key and signs the
// transfer server-side, so this is a single call (no client prepare/execute).
func (a *APIServerShim) SendCustodial(
	ctx context.Context,
	account *stack.Account,
	req *transfer.CustodialTransferRequest,
) (*transfer.ExecuteResponse, error) {
	msg := fmt.Sprintf("transfer:%d", time.Now().Unix())
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.ExecuteResponse
	if err := a.post(ctx, "/api/v2/transfer/custodial", sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// WithdrawCustodial claims back (withdraws) a pending or expired offer the custodial
// account sent, via POST /api/v2/transfer/outgoing/{contractID}/withdraw/custodial.
// account supplies the timed EIP-191 auth headers; the offer is identified by contractID.
func (a *APIServerShim) WithdrawCustodial(
	ctx context.Context,
	account *stack.Account,
	contractID string,
) (*transfer.ExecuteResponse, error) {
	msg := fmt.Sprintf("transfer:%d", time.Now().Unix())
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.ExecuteResponse
	path := "/api/v2/transfer/outgoing/" + contractID + "/withdraw/custodial"
	if err := a.post(ctx, path, sig, msg, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListOutgoingTransfers sends GET /api/v2/transfer/outgoing[?status=<status>] as
// account (caller from its bearer token). An empty status omits the filter (server
// defaults to all).
func (a *APIServerShim) ListOutgoingTransfers(
	ctx context.Context,
	account *stack.Account,
	status string,
) (*transfer.OutgoingTransfersList, error) {
	token, err := a.ensureToken(ctx, account)
	if err != nil {
		return nil, err
	}
	path := "/api/v2/transfer/outgoing"
	if status != "" {
		path += "?" + url.Values{"status": []string{status}}.Encode()
	}
	var resp transfer.OutgoingTransfersList
	if err := a.getBearer(ctx, path, token, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListCompletedTransfers sends GET /api/v2/transfer/completed as account (caller
// from its bearer token).
func (a *APIServerShim) ListCompletedTransfers(
	ctx context.Context,
	account *stack.Account,
) (*transfer.CompletedTransfersList, error) {
	token, err := a.ensureToken(ctx, account)
	if err != nil {
		return nil, err
	}
	var resp transfer.CompletedTransfersList
	if err := a.getBearer(ctx, "/api/v2/transfer/completed", token, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PrepareAcceptTransfer sends POST /api/v2/transfer/incoming/{contractID}/prepare.
func (a *APIServerShim) PrepareAcceptTransfer(
	ctx context.Context,
	account *stack.Account,
	contractID string,
	req *transfer.PrepareAcceptRequest,
) (*transfer.PrepareResponse, error) {
	msg := fmt.Sprintf("prepare-accept:%d", time.Now().Unix())
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.PrepareResponse
	path := "/api/v2/transfer/incoming/" + contractID + "/prepare"
	if err := a.post(ctx, path, sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ExecuteAcceptTransfer sends POST /api/v2/transfer/incoming/{contractID}/execute.
// contractID is embedded in req.TransferID for routing; the path uses the contractID
// extracted from the prepare response but the body carries the transfer_id + signature.
func (a *APIServerShim) ExecuteAcceptTransfer(
	ctx context.Context,
	account *stack.Account,
	contractID string,
	req *transfer.ExecuteRequest,
) (*transfer.ExecuteResponse, error) {
	msg := fmt.Sprintf("execute-accept:%d", time.Now().Unix())
	sig, err := util.SignEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.ExecuteResponse
	path := "/api/v2/transfer/incoming/" + contractID + "/execute"
	if err := a.post(ctx, path, sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
