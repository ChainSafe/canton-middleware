//go:build e2e

package shim

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/registry"
	"github.com/chainsafe/canton-middleware/pkg/transfer"
	"github.com/chainsafe/canton-middleware/pkg/user"
	"github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// APIServerShim implements stack.APIServer via HTTP.
type APIServerShim struct {
	endpoint string
	client   *http.Client
}

// NewAPIServer returns an APIServerShim for the api-server endpoint in the manifest.
func NewAPIServer(manifest *stack.ServiceManifest) *APIServerShim {
	return &APIServerShim{
		endpoint: manifest.APIHTTP,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *APIServerShim) Endpoint() string { return a.endpoint }

// Health returns nil when GET /health responds with 200.
func (a *APIServerShim) Health(ctx context.Context) error {
	return a.getOK(ctx, "/health")
}

// Register sends POST /register with EIP-191 signature and message.
func (a *APIServerShim) Register(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error) {
	var resp user.RegisterResponse
	if err := a.post(ctx, "/register", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PrepareTopology sends POST /register/prepare-topology.
func (a *APIServerShim) PrepareTopology(ctx context.Context, req *user.RegisterRequest) (*user.PrepareTopologyResponse, error) {
	var resp user.PrepareTopologyResponse
	if err := a.post(ctx, "/register/prepare-topology", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RegisterExternal sends POST /register with key_mode=external.
func (a *APIServerShim) RegisterExternal(ctx context.Context, req *user.RegisterRequest) (*user.RegisterResponse, error) {
	return a.Register(ctx, req)
}

// PrepareTransfer sends POST /api/v2/transfer/prepare with timed EIP-191 auth headers.
func (a *APIServerShim) PrepareTransfer(ctx context.Context, account *stack.Account, req *transfer.PrepareRequest) (*transfer.PrepareResponse, error) {
	msg := fmt.Sprintf("transfer:%d", time.Now().Unix())
	sig, err := signEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.PrepareResponse
	if err := a.postAuth(ctx, "/api/v2/transfer/prepare", sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ExecuteTransfer sends POST /api/v2/transfer/execute with timed EIP-191 auth headers.
func (a *APIServerShim) ExecuteTransfer(ctx context.Context, account *stack.Account, req *transfer.ExecuteRequest) (*transfer.ExecuteResponse, error) {
	msg := fmt.Sprintf("execute:%d", time.Now().Unix())
	sig, err := signEIP191(account.PrivateKey, msg)
	if err != nil {
		return nil, err
	}
	var resp transfer.ExecuteResponse
	if err := a.postAuth(ctx, "/api/v2/transfer/execute", sig, msg, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ERC20Balance calls POST /eth with an eth_call JSON-RPC request to read the
// ERC-20 balance of ownerAddr for tokenAddr through the api-server facade.
func (a *APIServerShim) ERC20Balance(ctx context.Context, tokenAddr, ownerAddr string) (string, error) {
	// Encode the balanceOf(address) call: selector 0x70a08231 + 32-byte zero-padded address.
	// common.LeftPadBytes ensures correct zero-padding (fmt.Sprintf %064s pads with spaces).
	addr := common.HexToAddress(ownerAddr)
	paddedOwner := hex.EncodeToString(common.LeftPadBytes(addr.Bytes(), 32))
	data := "0x70a08231" + paddedOwner

	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"method":  "eth_call",
		"params": []any{
			map[string]any{"to": tokenAddr, "data": data},
			"latest",
		},
		"id": 1,
	}
	var rpcResp struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := a.post(ctx, "/eth", rpcReq, &rpcResp); err != nil {
		return "", err
	}
	if rpcResp.Error != nil {
		return "", fmt.Errorf("eth_call error: %s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// TransferFactory sends POST /registry/transfer-instruction/v1/transfer-factory.
func (a *APIServerShim) TransferFactory(ctx context.Context) (*registry.TransferFactoryResponse, error) {
	var resp registry.TransferFactoryResponse
	if err := a.post(ctx, "/registry/transfer-instruction/v1/transfer-factory", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- internal helpers ---

func (a *APIServerShim) post(ctx context.Context, path string, body, out any) error {
	return a.do(ctx, path, "", "", body, out)
}

func (a *APIServerShim) postAuth(ctx context.Context, path, sig, msg string, body, out any) error {
	return a.do(ctx, path, sig, msg, body, out)
}

func (a *APIServerShim) do(ctx context.Context, path, sig, msg string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set("X-Signature", sig)
		req.Header.Set("X-Message", msg)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(raw))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response from %s: %w", path, err)
		}
	}
	return nil
}

func (a *APIServerShim) getOK(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint+path, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return nil
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
