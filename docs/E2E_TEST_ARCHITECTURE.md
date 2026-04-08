# Canton Middleware — E2E Testing Architecture

> **Inspired by:** [`op-devstack`](https://github.com/ethereum-optimism/optimism/tree/develop/op-devstack)
> **Test style:** [`op-acceptance-tests`](https://github.com/ethereum-optimism/optimism/blob/develop/op-acceptance-tests/tests/interop/reorgs/init_exec_msg_test.go)
> **Scope:** api-server + relayer + indexer end-to-end tests

---

## Table of Contents

1. [Design Goals](#1-design-goals)
2. [Architecture Overview](#2-architecture-overview)
3. [Package Structure](#3-package-structure)
4. [Layer 1 — Stack (Interfaces)](#4-layer-1--stack-interfaces)
5. [Layer 2 — Shim (Service Clients)](#5-layer-2--shim-service-clients)
6. [Layer 3 — Docker Orchestrator](#6-layer-3--docker-orchestrator)
7. [Layer 4 — System (Composition)](#7-layer-4--system-composition)
8. [Layer 5 — DSL (Test Utilities)](#8-layer-5--dsl-test-utilities)
9. [Layer 6 — Presets (Test Entry Points)](#9-layer-6--presets-test-entry-points)
10. [Writing Tests](#10-writing-tests)
11. [Full Directory Layout](#11-full-directory-layout)
12. [Docker Compose & Service Discovery](#12-docker-compose--service-discovery)
13. [Configuration & Environment Variables](#13-configuration--environment-variables)

---

## 1. Design Goals

- **Services in Docker.** Every dependency (Canton, Anvil, Postgres, OAuth2 mock) runs in Docker. No mocks at the service level.
- **Existing deployer reused.** The current `bootstrap-bridge.go` + `deploy-dars.canton` scripts run unchanged inside Docker at start-up.
- **Service discovery.** After Docker compose is healthy, a `ServiceManifest` maps every service to its `localhost:port`. Tests never hard-code addresses.
- **One-line test setup.** `sys := presets.NewFullStack(t)` — done. No docker logic in tests.
- **op-devstack layering.** `stack` (interfaces) → `shim` (clients) → `docker` (lifecycle) → `system` (composition) → `dsl` (helpers) → `presets` (entry points) → tests.
- **Package-level Docker start, test-level isolation.** Docker compose starts once per test _package_ via `TestMain`. Each test gets a fresh system reference but shares the running containers.
- **Indexer as a first-class service.** The indexer (`cmd/indexer`) streams Canton `TokenTransferEvent` contracts and exposes a read API at `/indexer/v1/admin`. E2E tests verify that bridge and transfer operations produce the correct indexed state (events, balances, total supply).

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        tests/e2e/tests/{api,bridge}/                     │
│                                                                          │
│  func TestDeposit(t *testing.T) {                                        │
│      sys := presets.NewFullStack(t)  ← one line to get everything        │
│      sys.DSL.RegisterUser(t, sys.Accounts.User1)                         │
│      txHash := sys.DSL.Deposit(t, sys.Accounts.User1, amount)            │
│      sys.DSL.WaitForCantonBalance(t, sys.Accounts.User1, "PROMPT", "100")│
│      sys.DSL.WaitForIndexerBalance(t, partyID, admin, "PROMPT", "100")   │
│  }                                                                       │
└────────────────────────────────┬─────────────────────────────────────────┘
                                  │ uses
┌─────────────────────────────────▼────────────────────────────────────────┐
│  tests/e2e/devstack/presets/       Layer 6: Presets                      │
│  NewFullStack(t), NewAPIStack(t), DoMain(m, opts...)                     │
│  Wires system + DSL, handles t.Cleanup()                                 │
└─────────────────────────────────┬────────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼───────────────────────────────────────┐
│  tests/e2e/devstack/dsl/           Layer 5: DSL                          │
│  RegisterUser, Deposit, Transfer, GetBalance, WaitForCantonBalance, ...  │
│  High-level operations composed from System methods                      │
└──────────────────────────────────┬───────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼──────────────────────────────────────┐
│  tests/e2e/devstack/system/        Layer 4: System                       │
│  System struct: holds all shims, the manifest, the DSL                   │
│  Built by: system.New(manifest)                                          │
└───────────────────────────────────┬──────────────────────────────────────┘
                                     │  shim.NewAnvil(manifest.AnvilRPC)
                                     │  shim.NewAPIServer(manifest.APIHTTP)
                                     │  shim.NewRelayer(manifest.RelayerHTTP)
                                     │  shim.NewIndexer(manifest.IndexerHTTP)
┌────────────────────────────────────▼─────────────────────────────────────┐
│  tests/e2e/devstack/shim/          Layer 2: Shims                        │
│  Wraps real service clients (go-ethereum, gRPC, HTTP) as stack interfaces│
└────────────────────────────────────┬─────────────────────────────────────┘
                                      │ implements
┌─────────────────────────────────────▼────────────────────────────────────┐
│  tests/e2e/devstack/stack/         Layer 1: Interfaces                   │
│  Anvil, Canton, APIServer, Relayer, Postgres (all interfaces)            │
└─────────────────────────────────────┬────────────────────────────────────┘
                                       │
┌──────────────────────────────────────▼───────────────────────────────────┐
│  tests/e2e/devstack/docker/        Layer 3: Orchestrator                 │
│  ComposeOrchestrator.Start()/Stop() → docker compose up/down             │
│  ServiceDiscovery.Manifest() → ServiceManifest{AnvilRPC, APIHTTP, ...}   │
└──────────────────────────────────────┬───────────────────────────────────┘
                                        │
                          ┌─────────────▼─────────────┐
                          │  Docker Compose            │
                          │  ┌─────────────────────┐  │
                          │  │ anvil       :8545    │  │
                          │  │ canton      :5011    │  │
                          │  │ postgres    :5432    │  │
                          │  │ oauth2-mock :8088    │  │
                          │  │ bootstrap   (init)   │  │
                          │  │ api-server  :8081    │  │
                          │  │ relayer     :8080    │  │
                          │  │ indexer     :8082    │  │
                          │  └─────────────────────┘  │
                          └───────────────────────────┘
```

---

## 3. Package Structure

```
tests/e2e/
├── devstack/                   ← the framework (never import from tests directly)
│   ├── stack/                  Layer 1 — interfaces + types
│   ├── shim/                   Layer 2 — concrete service clients
│   ├── docker/                 Layer 3 — compose lifecycle + service discovery
│   ├── system/                 Layer 4 — system composition
│   ├── dsl/                    Layer 5 — test DSL helpers
│   └── presets/                Layer 6 — test entry points
│
├── tests/                      ← actual test files
│   ├── api/                    api-server tests (registration, balance, transfer)
│   ├── bridge/                 relayer tests (deposit, withdrawal, reconciliation)
│   └── indexer/                indexer tests (event indexing, balance tracking, supply)
│
├── docker-compose.e2e.yaml     ← test-specific compose (ports published)
└── README.md
```

---

## 4. Layer 1 — Stack (Interfaces)

All service types are defined as interfaces here. Tests and DSL code only depend on
these interfaces, never on concrete implementations.

```go
// tests/e2e/devstack/stack/interfaces.go
package stack

import (
    "context"
    "math/big"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/ethclient"
)

// ----------------------------------------------------------------------------
// Anvil — local Ethereum node
// ----------------------------------------------------------------------------

type Anvil interface {
    // RPC returns a connected go-ethereum client.
    RPC() *ethclient.Client
    // ChainID returns the Anvil chain ID (31337 for local).
    ChainID() *big.Int
    // Endpoint returns the http RPC URL (e.g. "http://localhost:8545").
    Endpoint() string
    // ERC20Balance returns the on-chain ERC-20 balance of address for tokenAddr.
    ERC20Balance(ctx context.Context, tokenAddr, owner common.Address) (*big.Int, error)
    // ApproveAndDeposit approves the bridge contract then deposits amount.
    ApproveAndDeposit(ctx context.Context, key *Account, amount *big.Int) (common.Hash, error)
}

// ----------------------------------------------------------------------------
// Canton — ledger node
// ----------------------------------------------------------------------------

type Canton interface {
    // GRPCEndpoint returns the gRPC endpoint (e.g. "localhost:5011").
    GRPCEndpoint() string
    // HTTPEndpoint returns the HTTP endpoint (e.g. "http://localhost:5013").
    HTTPEndpoint() string
    // IsHealthy returns true when Canton is ready to accept commands.
    IsHealthy(ctx context.Context) bool
}

// ----------------------------------------------------------------------------
// APIServer — the canton-middleware api-server
// ----------------------------------------------------------------------------

type APIServer interface {
    // Endpoint returns the base HTTP URL.
    Endpoint() string
    // Register registers an EVM account with the api-server.
    Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)
    // GetBalance calls eth_call to get token balance for address.
    GetBalance(ctx context.Context, tokenAddr, ownerAddr string) (string, error)
    // Transfer calls eth_sendTransaction to transfer tokens.
    Transfer(ctx context.Context, req *TransferRequest) (string, error)
    // Health returns nil when the api-server is ready.
    Health(ctx context.Context) error
}

// ----------------------------------------------------------------------------
// Relayer — bridge relayer
// ----------------------------------------------------------------------------

type Relayer interface {
    // Endpoint returns the base HTTP URL.
    Endpoint() string
    // Health returns nil when the relayer is ready.
    Health(ctx context.Context) error
    // IsReady returns true when both Canton and Ethereum streams are synced.
    IsReady(ctx context.Context) bool
}

// ----------------------------------------------------------------------------
// Indexer — Canton token transfer event indexer
// ----------------------------------------------------------------------------

type Indexer interface {
    // Endpoint returns the base HTTP URL (e.g. "http://localhost:8082").
    Endpoint() string
    // Health returns nil when the indexer is ready and synced.
    Health(ctx context.Context) error

    // GetToken returns token state (total supply, holder count) for {admin, id}.
    GetToken(ctx context.Context, admin, id string) (*IndexerToken, error)
    // TotalSupply returns the current total supply decimal string for {admin, id}.
    TotalSupply(ctx context.Context, admin, id string) (string, error)
    // ListTokens returns a paginated list of all indexed tokens.
    ListTokens(ctx context.Context, page, limit int) (*IndexerTokenPage, error)

    // GetBalance returns the current balance for a canton party and instrument.
    GetBalance(ctx context.Context, partyID, admin, id string) (*IndexerBalance, error)
    // ListBalancesForParty returns all balances held by partyID.
    ListBalancesForParty(ctx context.Context, partyID string, page, limit int) (*IndexerBalancePage, error)

    // GetEvent returns a single indexed event by its unique contract ID.
    GetEvent(ctx context.Context, contractID string) (*IndexerEvent, error)
    // ListPartyEvents returns events where partyID is sender or receiver.
    // eventType filters by "MINT", "BURN", "TRANSFER", or "" for all.
    ListPartyEvents(ctx context.Context, partyID, eventType string, page, limit int) (*IndexerEventPage, error)
    // ListTokenEvents returns events for a specific instrument.
    ListTokenEvents(ctx context.Context, admin, id, eventType string, page, limit int) (*IndexerEventPage, error)
}

// ----------------------------------------------------------------------------
// Postgres — database connection
// ----------------------------------------------------------------------------

type Postgres interface {
    // DSN returns the postgres connection string.
    DSN() string
    // WhitelistAddress adds an EVM address to the whitelist table.
    WhitelistAddress(ctx context.Context, evmAddress string) error
    // GetUserByEVMAddress returns a user row or nil.
    GetUserByEVMAddress(ctx context.Context, evmAddress string) (*UserRow, error)
}
```

```go
// tests/e2e/devstack/stack/types.go
package stack

import "github.com/ethereum/go-ethereum/common"

// Account represents a test account (EVM key + address).
type Account struct {
    Address    common.Address
    PrivateKey string // hex, no 0x prefix
}

// ServiceManifest holds all localhost endpoints discovered after Docker compose is up.
type ServiceManifest struct {
    AnvilRPC      string // "http://localhost:8545"
    CantonGRPC    string // "localhost:5011"
    CantonHTTP    string // "http://localhost:5013"
    APIHTTP       string // "http://localhost:8081"
    RelayerHTTP   string // "http://localhost:8080"
    IndexerHTTP   string // "http://localhost:8082"
    OAuthHTTP     string // "http://localhost:8088"
    PostgresDSN   string // "postgres://postgres:p@ssw0rd@localhost:5432/erc20_api"

    // Contract addresses (extracted from deployer logs or env)
    PromptTokenAddr  string // "0x5FbDB..."
    BridgeAddr       string // "0xe7f172..."
    DemoTokenAddr    string // virtual: "0xDE3000..."

    // Canton instrument keys for indexer queries (populated after bootstrap)
    PromptInstrumentAdmin string // Canton party ID of the PROMPT token admin
    PromptInstrumentID    string // "PROMPT" — matches InstrumentKey.ID in indexer config
    DemoInstrumentAdmin   string // Canton party ID of the DEMO token admin
    DemoInstrumentID      string // "DEMO"
}

// IndexerToken mirrors indexer.Token — the ERC-20-like state tracked per instrument.
type IndexerToken struct {
    InstrumentAdmin string `json:"instrument_admin"`
    InstrumentID    string `json:"instrument_id"`
    Issuer          string `json:"issuer"`
    TotalSupply     string `json:"total_supply"`
    HolderCount     int64  `json:"holder_count"`
}

// IndexerBalance mirrors indexer.Balance — per-party holding for one instrument.
type IndexerBalance struct {
    PartyID         string `json:"party_id"`
    InstrumentAdmin string `json:"instrument_admin"`
    InstrumentID    string `json:"instrument_id"`
    Amount          string `json:"amount"`
}

// IndexerEvent mirrors indexer.ParsedEvent — a decoded TokenTransferEvent from the ledger.
type IndexerEvent struct {
    ContractID      string  `json:"contract_id"`
    TxID            string  `json:"tx_id"`
    InstrumentAdmin string  `json:"instrument_admin"`
    InstrumentID    string  `json:"instrument_id"`
    EventType       string  `json:"event_type"` // "MINT" | "BURN" | "TRANSFER"
    Amount          string  `json:"amount"`
    FromPartyID     *string `json:"from_party_id,omitempty"`
    ToPartyID       *string `json:"to_party_id,omitempty"`
    ExternalTxID    *string `json:"external_tx_id,omitempty"`
    ExternalAddress *string `json:"external_address,omitempty"`
    LedgerOffset    int64   `json:"ledger_offset"`
}

// IndexerTokenPage is the paginated response for token list queries.
type IndexerTokenPage struct {
    Items []*IndexerToken `json:"items"`
    Total int64           `json:"total"`
    Page  int             `json:"page"`
    Limit int             `json:"limit"`
}

// IndexerBalancePage is the paginated response for balance list queries.
type IndexerBalancePage struct {
    Items []*IndexerBalance `json:"items"`
    Total int64             `json:"total"`
    Page  int               `json:"page"`
    Limit int               `json:"limit"`
}

// IndexerEventPage is the paginated response for event list queries.
type IndexerEventPage struct {
    Items []*IndexerEvent `json:"items"`
    Total int64           `json:"total"`
    Page  int             `json:"page"`
    Limit int             `json:"limit"`
}

// Preconfigured Anvil test accounts (deterministic from mnemonic)
var (
    AnvilAccount0 = &Account{
        Address:    common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
        PrivateKey: "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
    }
    AnvilAccount1 = &Account{
        Address:    common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"),
        PrivateKey: "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
    }
)

type RegisterRequest struct {
    EVMAddress string
    Signature  string
    Message    string
}

type RegisterResponse struct {
    EVMAddress    string
    CantonPartyID string
    Fingerprint   string
    MappingCID    string
}

type TransferRequest struct {
    From      common.Address
    To        common.Address
    Amount    string
    TokenAddr string
}

type UserRow struct {
    EVMAddress    string
    CantonPartyID string
    Fingerprint   string
}
```

---

## 5. Layer 2 — Shim (Service Clients)

Each shim wraps a real network client and implements the stack interface.

```go
// tests/e2e/devstack/shim/anvil.go
package shim

import (
    "context"
    "math/big"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/ethclient"
)

type anvilShim struct {
    endpoint string
    chainID  *big.Int
    client   *ethclient.Client
    // ABIs for PromptToken, CantonBridge (loaded from contracts/)
    promptTokenABI abi.ABI
    bridgeABI      abi.ABI
    bridgeAddr     common.Address
    tokenAddr      common.Address
}

func NewAnvil(manifest *stack.ServiceManifest) (stack.Anvil, error) {
    client, err := ethclient.Dial(manifest.AnvilRPC)
    if err != nil {
        return nil, fmt.Errorf("dial anvil: %w", err)
    }
    chainID, err := client.ChainID(context.Background())
    if err != nil {
        return nil, fmt.Errorf("get chain id: %w", err)
    }
    // load ABIs from embedded contract artifacts
    pABI, _ := loadABI("PromptToken")
    bABI, _ := loadABI("CantonBridge")
    return &anvilShim{
        endpoint:       manifest.AnvilRPC,
        chainID:        chainID,
        client:         client,
        promptTokenABI: pABI,
        bridgeABI:      bABI,
        bridgeAddr:     common.HexToAddress(manifest.BridgeAddr),
        tokenAddr:      common.HexToAddress(manifest.PromptTokenAddr),
    }, nil
}

func (a *anvilShim) RPC() *ethclient.Client { return a.client }
func (a *anvilShim) ChainID() *big.Int      { return a.chainID }
func (a *anvilShim) Endpoint() string       { return a.endpoint }

func (a *anvilShim) ERC20Balance(ctx context.Context, tokenAddr, owner common.Address) (*big.Int, error) {
    // call balanceOf(owner) on tokenAddr using go-ethereum CallContract
    callData, _ := a.promptTokenABI.Pack("balanceOf", owner)
    result, err := a.client.CallContract(ctx, ethereum.CallMsg{
        To:   &tokenAddr,
        Data: callData,
    }, nil)
    if err != nil {
        return nil, err
    }
    out, _ := a.promptTokenABI.Unpack("balanceOf", result)
    return out[0].(*big.Int), nil
}

func (a *anvilShim) ApproveAndDeposit(ctx context.Context, acc *stack.Account, amount *big.Int) (common.Hash, error) {
    key, _ := crypto.HexToECDSA(acc.PrivateKey)
    signer := types.LatestSignerForChainID(a.chainID)
    opts := bind.NewKeyedTransactor(key)
    opts.Signer = func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
        return types.SignTx(tx, signer, key)
    }
    // 1. approve bridge to spend tokens
    // 2. deposit on bridge contract
    // returns deposit tx hash
    return depositOnBridge(ctx, a.client, opts, a.tokenAddr, a.bridgeAddr, amount)
}
```

```go
// tests/e2e/devstack/shim/apiserver.go
package shim

type apiServerShim struct {
    endpoint   string
    httpClient *http.Client
}

func NewAPIServer(manifest *stack.ServiceManifest) stack.APIServer {
    return &apiServerShim{
        endpoint:   manifest.APIHTTP,
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}

func (a *apiServerShim) Register(ctx context.Context, req *stack.RegisterRequest) (*stack.RegisterResponse, error) {
    body, _ := json.Marshal(req)
    resp, err := a.httpClient.Post(a.endpoint+"/register", "application/json", bytes.NewReader(body))
    if err != nil {
        return nil, fmt.Errorf("register: %w", err)
    }
    defer resp.Body.Close()
    var out stack.RegisterResponse
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, err
    }
    return &out, nil
}

func (a *apiServerShim) GetBalance(ctx context.Context, tokenAddr, ownerAddr string) (string, error) {
    // JSON-RPC eth_call to /eth endpoint
    // callData = ABI.pack("balanceOf", ownerAddr)
    // returns decoded decimal string
    rpc := newJSONRPC(a.endpoint + "/eth")
    return rpc.EthCall_balanceOf(ctx, tokenAddr, ownerAddr)
}

func (a *apiServerShim) Health(ctx context.Context) error {
    resp, err := a.httpClient.Get(a.endpoint + "/health")
    if err != nil || resp.StatusCode != 200 {
        return fmt.Errorf("api-server not ready")
    }
    return nil
}
```

```go
// tests/e2e/devstack/shim/indexer.go
package shim

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

type indexerShim struct {
    endpoint   string
    httpClient *http.Client
}

func NewIndexer(manifest *stack.ServiceManifest) stack.Indexer {
    return &indexerShim{
        endpoint:   manifest.IndexerHTTP,
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}

func (ix *indexerShim) Endpoint() string { return ix.endpoint }

func (ix *indexerShim) Health(ctx context.Context) error {
    resp, err := ix.httpClient.Get(ix.endpoint + "/health")
    if err != nil || resp.StatusCode != 200 {
        return fmt.Errorf("indexer not ready")
    }
    return nil
}

func (ix *indexerShim) GetToken(ctx context.Context, admin, id string) (*stack.IndexerToken, error) {
    return getJSON[stack.IndexerToken](ix.httpClient, fmt.Sprintf(
        "%s/indexer/v1/admin/tokens/%s/%s", ix.endpoint, admin, id,
    ))
}

func (ix *indexerShim) TotalSupply(ctx context.Context, admin, id string) (string, error) {
    var out struct{ TotalSupply string `json:"total_supply"` }
    raw, err := getJSON[struct{ TotalSupply string `json:"total_supply"` }](ix.httpClient, fmt.Sprintf(
        "%s/indexer/v1/admin/tokens/%s/%s/supply", ix.endpoint, admin, id,
    ))
    if err != nil {
        return "", err
    }
    return raw.TotalSupply, nil
    _ = out
}

func (ix *indexerShim) ListTokens(ctx context.Context, page, limit int) (*stack.IndexerTokenPage, error) {
    return getJSON[stack.IndexerTokenPage](ix.httpClient, fmt.Sprintf(
        "%s/indexer/v1/admin/tokens?page=%d&limit=%d", ix.endpoint, page, limit,
    ))
}

func (ix *indexerShim) GetBalance(ctx context.Context, partyID, admin, id string) (*stack.IndexerBalance, error) {
    return getJSON[stack.IndexerBalance](ix.httpClient, fmt.Sprintf(
        "%s/indexer/v1/admin/parties/%s/balances/%s/%s", ix.endpoint, partyID, admin, id,
    ))
}

func (ix *indexerShim) ListBalancesForParty(ctx context.Context, partyID string, page, limit int) (*stack.IndexerBalancePage, error) {
    return getJSON[stack.IndexerBalancePage](ix.httpClient, fmt.Sprintf(
        "%s/indexer/v1/admin/parties/%s/balances?page=%d&limit=%d", ix.endpoint, partyID, page, limit,
    ))
}

func (ix *indexerShim) GetEvent(ctx context.Context, contractID string) (*stack.IndexerEvent, error) {
    return getJSON[stack.IndexerEvent](ix.httpClient, fmt.Sprintf(
        "%s/indexer/v1/admin/events/%s", ix.endpoint, contractID,
    ))
}

func (ix *indexerShim) ListPartyEvents(ctx context.Context, partyID, eventType string, page, limit int) (*stack.IndexerEventPage, error) {
    url := fmt.Sprintf("%s/indexer/v1/admin/parties/%s/events?page=%d&limit=%d", ix.endpoint, partyID, page, limit)
    if eventType != "" {
        url += "&event_type=" + eventType
    }
    return getJSON[stack.IndexerEventPage](ix.httpClient, url)
}

func (ix *indexerShim) ListTokenEvents(ctx context.Context, admin, id, eventType string, page, limit int) (*stack.IndexerEventPage, error) {
    url := fmt.Sprintf("%s/indexer/v1/admin/tokens/%s/%s/events?page=%d&limit=%d", ix.endpoint, admin, id, page, limit)
    if eventType != "" {
        url += "&event_type=" + eventType
    }
    return getJSON[stack.IndexerEventPage](ix.httpClient, url)
}

// getJSON is a generic helper that GETs url and decodes the JSON body into T.
func getJSON[T any](client *http.Client, url string) (*T, error) {
    resp, err := client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("GET %s: %w", url, err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
    }
    var out T
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("decode response from %s: %w", url, err)
    }
    return &out, nil
}
```

```go
// tests/e2e/devstack/shim/postgres.go
package shim

type postgresShim struct {
    dsn string
    db  *sql.DB
}

func NewPostgres(manifest *stack.ServiceManifest) (stack.Postgres, error) {
    db, err := sql.Open("postgres", manifest.PostgresDSN)
    if err != nil {
        return nil, err
    }
    return &postgresShim{dsn: manifest.PostgresDSN, db: db}, nil
}

func (p *postgresShim) WhitelistAddress(ctx context.Context, evmAddress string) error {
    _, err := p.db.ExecContext(ctx,
        `INSERT INTO whitelist (evm_address) VALUES ($1) ON CONFLICT DO NOTHING`,
        strings.ToLower(evmAddress),
    )
    return err
}

func (p *postgresShim) GetUserByEVMAddress(ctx context.Context, evmAddress string) (*stack.UserRow, error) {
    row := p.db.QueryRowContext(ctx,
        `SELECT evm_address, canton_party_id, fingerprint FROM users WHERE evm_address = $1`,
        strings.ToLower(evmAddress),
    )
    u := new(stack.UserRow)
    err := row.Scan(&u.EVMAddress, &u.CantonPartyID, &u.Fingerprint)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    return u, err
}
```

---

## 6. Layer 3 — Docker Orchestrator

### ComposeOrchestrator

Manages the docker compose lifecycle. Wraps `docker compose up/down` commands.
Runs the existing deployer / bootstrap as a compose service (no changes to existing scripts).

```go
// tests/e2e/devstack/docker/compose.go
package docker

import (
    "context"
    "fmt"
    "os/exec"
    "time"
)

type ComposeOrchestrator struct {
    composeFile string    // path to docker-compose.e2e.yaml
    projectName string    // e.g. "canton-e2e"
    env         []string  // extra env vars passed to compose
}

func NewOrchestrator(composeFile, projectName string) *ComposeOrchestrator {
    return &ComposeOrchestrator{
        composeFile: composeFile,
        projectName: projectName,
    }
}

// Start runs `docker compose up -d` and waits for all healthchecks to pass.
// This is called once per test package in TestMain.
func (o *ComposeOrchestrator) Start(ctx context.Context) error {
    cmd := o.cmd("up", "--build", "--wait", "--remove-orphans")
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("compose up failed: %w\n%s", err, out)
    }
    return nil
}

// Stop runs `docker compose down -v` (also removes volumes for clean state).
func (o *ComposeOrchestrator) Stop(ctx context.Context) error {
    cmd := o.cmd("down", "-v", "--remove-orphans")
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("compose down failed: %w\n%s", err, out)
    }
    return nil
}

func (o *ComposeOrchestrator) cmd(args ...string) *exec.Cmd {
    base := []string{
        "compose",
        "-f", o.composeFile,
        "-p", o.projectName,
    }
    cmd := exec.Command("docker", append(base, args...)...)
    cmd.Env = append(os.Environ(), o.env...)
    return cmd
}
```

### ServiceDiscovery

After compose is up, discovery reads the published ports from container inspect output
and builds the `ServiceManifest`.

```go
// tests/e2e/devstack/docker/discovery.go
package docker

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "strings"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

type ServiceDiscovery struct {
    projectName string
}

func NewServiceDiscovery(projectName string) *ServiceDiscovery {
    return &ServiceDiscovery{projectName: projectName}
}

// Manifest inspects running containers and builds the service manifest.
// It reads published host ports for each service so tests connect via localhost.
func (d *ServiceDiscovery) Manifest(ctx context.Context) (*stack.ServiceManifest, error) {
    // Read contract addresses from deployer output (written to a shared volume file)
    addrs, err := d.readDeployerOutput()
    if err != nil {
        return nil, fmt.Errorf("read deployer output: %w", err)
    }

    return &stack.ServiceManifest{
        AnvilRPC:     "http://localhost:" + d.publishedPort("anvil", "8545"),
        CantonGRPC:   "localhost:" + d.publishedPort("canton", "5011"),
        CantonHTTP:   "http://localhost:" + d.publishedPort("canton", "5013"),
        APIHTTP:      "http://localhost:" + d.publishedPort("api-server", "8081"),
        RelayerHTTP:  "http://localhost:" + d.publishedPort("relayer", "8080"),
        IndexerHTTP:  "http://localhost:" + d.publishedPort("indexer", "8082"),
        OAuthHTTP:    "http://localhost:" + d.publishedPort("oauth2-mock", "8088"),
        PostgresDSN:  fmt.Sprintf("postgres://postgres:p@ssw0rd@localhost:%s/erc20_api",
            d.publishedPort("postgres", "5432")),
        PromptTokenAddr: addrs.PromptToken,
        BridgeAddr:      addrs.CantonBridge,
        DemoTokenAddr:   "0xDE30000000000000000000000000000000000001",
        // Canton instrument keys for indexer queries — sourced from deployer output or env.
        PromptInstrumentAdmin: addrs.PromptInstrumentAdmin,
        PromptInstrumentID:    "PROMPT",
        DemoInstrumentAdmin:   addrs.DemoInstrumentAdmin,
        DemoInstrumentID:      "DEMO",
    }, nil
}

// publishedPort returns the host port bound to containerPort for serviceName.
// Uses `docker compose port <service> <containerPort>`.
func (d *ServiceDiscovery) publishedPort(service, containerPort string) string {
    out, err := exec.Command("docker", "compose",
        "-p", d.projectName,
        "port", service, containerPort,
    ).Output()
    if err != nil {
        panic(fmt.Sprintf("could not get port for %s:%s: %v", service, containerPort, err))
    }
    // output is "0.0.0.0:XXXXX" — extract port
    parts := strings.Split(strings.TrimSpace(string(out)), ":")
    return parts[len(parts)-1]
}

// readDeployerOutput reads contract addresses and Canton party IDs written by the
// bootstrap container to a known location (mounted volume or env file).
type deployerOutput struct {
    PromptToken          string `json:"prompt_token"`
    CantonBridge         string `json:"canton_bridge"`
    // Canton instrument admin parties — needed by indexer queries.
    // Written by the bootstrap script after deploying the DAML token configs.
    PromptInstrumentAdmin string `json:"prompt_instrument_admin"` // PROMPT token admin canton_party_id
    DemoInstrumentAdmin   string `json:"demo_instrument_admin"`   // DEMO token admin canton_party_id
}

func (d *ServiceDiscovery) readDeployerOutput() (*deployerOutput, error) {
    // Bootstrap container writes /tmp/e2e-deploy.json on the shared volume
    out, err := exec.Command("docker", "compose",
        "-p", d.projectName,
        "run", "--rm", "--no-deps", "bootstrap",
        "cat", "/tmp/e2e-deploy.json",
    ).Output()
    if err != nil {
        // Fall back to env-embedded defaults (for anvil deterministic deploys)
        return &deployerOutput{
            PromptToken:  "0x5FbDB2315678afecb367f032d93F642f64180aa3",
            CantonBridge: "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512",
        }, nil
    }
    var result deployerOutput
    return &result, json.Unmarshal(out, &result)
}
```

---

## 7. Layer 4 — System (Composition)

The `System` struct holds all shims and is created from a `ServiceManifest`.
Tests access services through it.

```go
// tests/e2e/devstack/system/system.go
package system

import (
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/dsl"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/shim"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

// System is the composition of all running services, accessible from tests.
// It intentionally mirrors the op-devstack System pattern.
type System struct {
    // Service shims — access live services
    Anvil     stack.Anvil
    Canton    stack.Canton
    APIServer stack.APIServer
    Relayer   stack.Relayer
    Indexer   stack.Indexer
    Postgres  stack.Postgres

    // Manifest — raw service endpoints (for lower-level access if needed)
    Manifest *stack.ServiceManifest

    // Accounts — deterministic Anvil test accounts
    Accounts struct {
        User1 *stack.Account // AnvilAccount0 — has initial PROMPT supply
        User2 *stack.Account // AnvilAccount1
    }

    // DSL — high-level test operations
    DSL *dsl.DSL
}

// New builds a System from a ServiceManifest.
// All shims are initialized with live service connections.
func New(t *testing.T, manifest *stack.ServiceManifest) *System {
    t.Helper()

    anvil, err := shim.NewAnvil(manifest)
    if err != nil {
        t.Fatalf("init anvil shim: %v", err)
    }
    canton := shim.NewCanton(manifest)
    api := shim.NewAPIServer(manifest)
    relayer := shim.NewRelayer(manifest)
    indexer := shim.NewIndexer(manifest)
    pg, err := shim.NewPostgres(manifest)
    if err != nil {
        t.Fatalf("init postgres shim: %v", err)
    }

    sys := &System{
        Anvil:     anvil,
        Canton:    canton,
        APIServer: api,
        Relayer:   relayer,
        Indexer:   indexer,
        Postgres:  pg,
        Manifest:  manifest,
    }
    sys.Accounts.User1 = stack.AnvilAccount0
    sys.Accounts.User2 = stack.AnvilAccount1
    sys.DSL = dsl.New(sys.Anvil, sys.APIServer, sys.Relayer, sys.Indexer, sys.Postgres, manifest)

    return sys
}
```

---

## 8. Layer 5 — DSL (Test Utilities)

High-level operations that compose multiple service calls into meaningful test steps.
Mirrors the `dsl` package from op-devstack.

```go
// tests/e2e/devstack/dsl/dsl.go
package dsl

import (
    "context"
    "math/big"
    "testing"
    "time"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
    "github.com/stretchr/testify/require"
)

// DSL provides high-level test operations.
// Each method is intentionally test-friendly (takes *testing.T, calls require on failure).
type DSL struct {
    anvil    stack.Anvil
    api      stack.APIServer
    relayer  stack.Relayer
    indexer  stack.Indexer
    postgres stack.Postgres
    manifest *stack.ServiceManifest
}

func New(
    anvil stack.Anvil,
    api stack.APIServer,
    relayer stack.Relayer,
    indexer stack.Indexer,
    pg stack.Postgres,
    manifest *stack.ServiceManifest,
) *DSL {
    return &DSL{
        anvil: anvil, api: api, relayer: relayer, indexer: indexer, postgres: pg, manifest: manifest,
    }
}

// RegisterUser whitelists and registers an EVM account with the api-server.
// Returns the registered user info. Fails the test on any error.
func (d *DSL) RegisterUser(t *testing.T, acc *stack.Account) *stack.RegisterResponse {
    t.Helper()
    ctx := context.Background()

    // 1. Whitelist in postgres (prerequisite for registration)
    require.NoError(t, d.postgres.WhitelistAddress(ctx, acc.Address.Hex()))

    // 2. Sign registration message with EVM key (EIP-191)
    msg := "Register with Canton Middleware"
    sig := signEIP191(acc.PrivateKey, msg)

    // 3. Call api-server /register
    resp, err := d.api.Register(ctx, &stack.RegisterRequest{
        EVMAddress: acc.Address.Hex(),
        Signature:  sig,
        Message:    msg,
    })
    require.NoError(t, err, "register user %s", acc.Address.Hex())
    require.NotEmpty(t, resp.CantonPartyID)
    return resp
}

// Deposit approves and deposits amount of PROMPT tokens to the Canton bridge.
// Returns the Ethereum transaction hash.
func (d *DSL) Deposit(t *testing.T, acc *stack.Account, amount *big.Int) string {
    t.Helper()
    txHash, err := d.anvil.ApproveAndDeposit(context.Background(), acc, amount)
    require.NoError(t, err, "deposit %s wei for %s", amount, acc.Address.Hex())
    return txHash.Hex()
}

// GetBalance returns the token balance (as decimal string) for acc.
// tokenAddr is the ERC-20 contract address on the api-server config.
func (d *DSL) GetBalance(t *testing.T, acc *stack.Account, tokenAddr string) string {
    t.Helper()
    bal, err := d.api.GetBalance(context.Background(), tokenAddr, acc.Address.Hex())
    require.NoError(t, err)
    return bal
}

// Transfer sends amount tokens from acc to toAddr via api-server.
func (d *DSL) Transfer(t *testing.T, from *stack.Account, toAddr string, amount string, tokenAddr string) {
    t.Helper()
    _, err := d.api.Transfer(context.Background(), &stack.TransferRequest{
        From:      from.Address,
        To:        common.HexToAddress(toAddr),
        Amount:    amount,
        TokenAddr: tokenAddr,
    })
    require.NoError(t, err)
}

// WaitForCantonBalance polls the api-server until the balance for acc equals expected,
// or fails the test after timeout.
func (d *DSL) WaitForCantonBalance(t *testing.T, acc *stack.Account, tokenAddr, expected string) {
    t.Helper()
    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) {
        bal := d.GetBalance(t, acc, tokenAddr)
        if balanceEquals(bal, expected) {
            return
        }
        time.Sleep(2 * time.Second)
    }
    t.Fatalf("timeout waiting for balance %s for %s (token %s)", expected, acc.Address.Hex(), tokenAddr)
}

// WaitForRelayerReady polls the relayer health endpoint until it reports ready.
func (d *DSL) WaitForRelayerReady(t *testing.T) {
    t.Helper()
    deadline := time.Now().Add(60 * time.Second)
    for time.Now().Before(deadline) {
        if d.relayer.IsReady(context.Background()) {
            return
        }
        time.Sleep(2 * time.Second)
    }
    t.Fatal("timeout waiting for relayer to be ready")
}

// MintDEMO calls the api-server's internal mint endpoint (or bootstrap script)
// to mint DEMO tokens for acc. Used as test fixture setup.
func (d *DSL) MintDEMO(t *testing.T, acc *stack.Account, amount string) {
    t.Helper()
    // Calls bootstrap-demo.go equivalent logic via API or direct Canton client
    // Implementation depends on whether api-server exposes an admin mint endpoint
}

// ---------------------------------------------------------------------------
// Indexer DSL helpers
// ---------------------------------------------------------------------------

// GetIndexerBalance returns the current indexed balance for partyID and instrument {admin, id}.
// Fails the test if the indexer returns an error.
func (d *DSL) GetIndexerBalance(t *testing.T, partyID, admin, id string) string {
    t.Helper()
    b, err := d.indexer.GetBalance(context.Background(), partyID, admin, id)
    require.NoError(t, err, "get indexer balance for party %s instrument %s/%s", partyID, admin, id)
    return b.Amount
}

// WaitForIndexerBalance polls the indexer until the balance for partyID equals expected,
// or fails the test after timeout. Use after any operation that should produce an
// indexed balance change (deposit → MINT, transfer → TRANSFER, withdrawal → BURN).
func (d *DSL) WaitForIndexerBalance(t *testing.T, partyID, admin, id, expected string) {
    t.Helper()
    deadline := time.Now().Add(60 * time.Second)
    for time.Now().Before(deadline) {
        b, err := d.indexer.GetBalance(context.Background(), partyID, admin, id)
        if err == nil && balanceEquals(b.Amount, expected) {
            return
        }
        time.Sleep(2 * time.Second)
    }
    t.Fatalf("timeout waiting for indexer balance %s for party %s instrument %s/%s", expected, partyID, admin, id)
}

// GetIndexerTotalSupply returns the current indexed total supply for instrument {admin, id}.
func (d *DSL) GetIndexerTotalSupply(t *testing.T, admin, id string) string {
    t.Helper()
    supply, err := d.indexer.TotalSupply(context.Background(), admin, id)
    require.NoError(t, err, "get indexer total supply for %s/%s", admin, id)
    return supply
}

// WaitForIndexerTotalSupply polls the indexer until TotalSupply for {admin, id} equals expected.
func (d *DSL) WaitForIndexerTotalSupply(t *testing.T, admin, id, expected string) {
    t.Helper()
    deadline := time.Now().Add(60 * time.Second)
    for time.Now().Before(deadline) {
        supply, err := d.indexer.TotalSupply(context.Background(), admin, id)
        if err == nil && balanceEquals(supply, expected) {
            return
        }
        time.Sleep(2 * time.Second)
    }
    t.Fatalf("timeout waiting for indexer total supply %s for %s/%s", expected, admin, id)
}

// GetIndexerEvent returns the indexed event with the given contract ID.
// contractID is the unique DAML TokenTransferEvent contract ID (idempotency key).
func (d *DSL) GetIndexerEvent(t *testing.T, contractID string) *stack.IndexerEvent {
    t.Helper()
    e, err := d.indexer.GetEvent(context.Background(), contractID)
    require.NoError(t, err, "get indexer event %s", contractID)
    return e
}

// WaitForIndexerEvent polls the indexer until an event with the given contractID appears.
// Returns the event once indexed. Use to verify that a specific DAML contract was processed.
func (d *DSL) WaitForIndexerEvent(t *testing.T, contractID string) *stack.IndexerEvent {
    t.Helper()
    deadline := time.Now().Add(60 * time.Second)
    for time.Now().Before(deadline) {
        e, err := d.indexer.GetEvent(context.Background(), contractID)
        if err == nil && e != nil {
            return e
        }
        time.Sleep(2 * time.Second)
    }
    t.Fatalf("timeout waiting for indexer event contractID=%s", contractID)
    return nil
}

// ListIndexerPartyEvents returns all indexed events for partyID, optionally filtered
// by eventType ("MINT", "BURN", "TRANSFER", or "" for all).
func (d *DSL) ListIndexerPartyEvents(t *testing.T, partyID, eventType string) []*stack.IndexerEvent {
    t.Helper()
    page, err := d.indexer.ListPartyEvents(context.Background(), partyID, eventType, 1, 200)
    require.NoError(t, err, "list indexer events for party %s", partyID)
    return page.Items
}
```

---

## 9. Layer 6 — Presets (Test Entry Points)

One-line setup functions for tests. Presets manage Docker compose lifecycle at package
level and create per-test System instances.

```go
// tests/e2e/devstack/presets/presets.go
package presets

import (
    "context"
    "os"
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/docker"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/system"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/stack"
)

const (
    defaultComposeFile = "../../docker-compose.e2e.yaml"
    defaultProjectName = "canton-e2e"
)

// sharedManifest is populated by DoMain once and reused by all tests in the package.
var sharedManifest *stack.ServiceManifest

// DoMain must be called from TestMain in every test package.
// It starts docker compose once for the whole package, runs tests, then tears down.
//
// Usage:
//
//	func TestMain(m *testing.M) {
//	    presets.DoMain(m)
//	}
func DoMain(m *testing.M, opts ...Option) {
    cfg := applyOptions(opts)

    orch := docker.NewOrchestrator(cfg.composeFile, cfg.projectName)
    disc := docker.NewServiceDiscovery(cfg.projectName)

    ctx := context.Background()

    if err := orch.Start(ctx); err != nil {
        panic("failed to start docker compose: " + err.Error())
    }

    manifest, err := disc.Manifest(ctx)
    if err != nil {
        _ = orch.Stop(ctx)
        panic("service discovery failed: " + err.Error())
    }
    sharedManifest = manifest

    code := m.Run()

    if os.Getenv("E2E_KEEP_SERVICES") != "true" {
        _ = orch.Stop(ctx)
    }

    os.Exit(code)
}

// NewFullStack returns a System with all services wired: Anvil + Canton + Postgres
// + APIServer + Relayer. Use this for bridge tests.
func NewFullStack(t *testing.T) *system.System {
    t.Helper()
    if sharedManifest == nil {
        t.Fatal("presets.DoMain was not called in TestMain")
    }
    return system.New(t, sharedManifest)
}

// NewAPIStack returns a System with APIServer + Postgres only (no relayer, no Anvil).
// Use this for api-server unit-style tests.
func NewAPIStack(t *testing.T) *system.System {
    t.Helper()
    if sharedManifest == nil {
        t.Fatal("presets.DoMain was not called in TestMain")
    }
    return system.New(t, sharedManifest)
}

// NewIndexerStack returns a System with APIServer + Indexer + Canton + Postgres.
// Use this for indexer tests that drive events via the api-server and check indexed state.
// The full bridge (Anvil + Relayer) is not needed for DEMO-token (non-bridge) indexer tests.
func NewIndexerStack(t *testing.T) *system.System {
    t.Helper()
    if sharedManifest == nil {
        t.Fatal("presets.DoMain was not called in TestMain")
    }
    return system.New(t, sharedManifest)
}

// NewFullStack returns a System with all services including the Indexer.
// Use this for bridge + indexer tests that verify the full mint→index→burn→index lifecycle.
func NewFullStack(t *testing.T) *system.System {
    t.Helper()
    if sharedManifest == nil {
        t.Fatal("presets.DoMain was not called in TestMain")
    }
    return system.New(t, sharedManifest)
}

// Option configures DoMain behaviour.
type Option func(*presetConfig)

func WithComposeFile(path string) Option {
    return func(c *presetConfig) { c.composeFile = path }
}

func WithProjectName(name string) Option {
    return func(c *presetConfig) { c.projectName = name }
}

type presetConfig struct {
    composeFile string
    projectName string
}

func applyOptions(opts []Option) *presetConfig {
    cfg := &presetConfig{
        composeFile: envOr("E2E_COMPOSE_FILE", defaultComposeFile),
        projectName: envOr("E2E_PROJECT_NAME", defaultProjectName),
    }
    for _, o := range opts {
        o(cfg)
    }
    return cfg
}
```

---

## 10. Writing Tests

Tests import only `presets` and use `sys.*` / `sys.DSL.*`.
No docker, no HTTP, no gRPC in test files.

### API Server Tests

```go
// tests/e2e/tests/api/main_test.go
package api_test

import (
    "testing"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

func TestMain(m *testing.M) {
    // Starts docker compose once for all tests in this package.
    presets.DoMain(m)
}
```

```go
// tests/e2e/tests/api/register_test.go
package api_test

import (
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

func TestRegister_NewUser_Success(t *testing.T) {
    sys := presets.NewAPIStack(t)

    resp := sys.DSL.RegisterUser(t, sys.Accounts.User1)

    require.Equal(t, sys.Accounts.User1.Address.Hex(), resp.EVMAddress)
    require.NotEmpty(t, resp.CantonPartyID)
    require.NotEmpty(t, resp.Fingerprint)
    require.NotEmpty(t, resp.MappingCID)

    // Verify user is in DB
    user, err := sys.Postgres.GetUserByEVMAddress(t.Context(), resp.EVMAddress)
    require.NoError(t, err)
    require.NotNil(t, user)
    require.Equal(t, resp.CantonPartyID, user.CantonPartyID)
}

func TestRegister_Duplicate_Idempotent(t *testing.T) {
    sys := presets.NewAPIStack(t)

    // First registration
    resp1 := sys.DSL.RegisterUser(t, sys.Accounts.User1)
    // Second registration — should succeed (idempotent) or return existing
    resp2 := sys.DSL.RegisterUser(t, sys.Accounts.User1)

    require.Equal(t, resp1.CantonPartyID, resp2.CantonPartyID)
}
```

```go
// tests/e2e/tests/api/balance_test.go
package api_test

import (
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

func TestGetBalance_UnregisteredUser_ReturnsZero(t *testing.T) {
    sys := presets.NewAPIStack(t)

    // Unregistered address should return "0", not error
    bal := sys.DSL.GetBalance(t, sys.Accounts.User2, sys.Manifest.PromptTokenAddr)
    require.Equal(t, "0", bal)
}

func TestGetBalance_AfterMintDEMO(t *testing.T) {
    sys := presets.NewAPIStack(t)

    sys.DSL.RegisterUser(t, sys.Accounts.User1)
    sys.DSL.MintDEMO(t, sys.Accounts.User1, "500")

    bal := sys.DSL.GetBalance(t, sys.Accounts.User1, sys.Manifest.DemoTokenAddr)
    require.Equal(t, "500", bal)
}

func TestGetTokenMetadata(t *testing.T) {
    sys := presets.NewAPIStack(t)

    // name, symbol, decimals, totalSupply should all be accessible without auth
    name, err := sys.APIServer.GetTokenName(t.Context(), sys.Manifest.PromptTokenAddr)
    require.NoError(t, err)
    require.Equal(t, "Prompt", name)

    decimals, err := sys.APIServer.GetTokenDecimals(t.Context(), sys.Manifest.PromptTokenAddr)
    require.NoError(t, err)
    require.Equal(t, uint8(18), decimals)
}
```

```go
// tests/e2e/tests/api/transfer_test.go
package api_test

import (
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

func TestTransfer_DEMO_BetweenUsers(t *testing.T) {
    sys := presets.NewAPIStack(t)

    // Setup: register both users and mint DEMO for user1
    sys.DSL.RegisterUser(t, sys.Accounts.User1)
    sys.DSL.RegisterUser(t, sys.Accounts.User2)
    sys.DSL.MintDEMO(t, sys.Accounts.User1, "100")

    // Action: transfer 30 DEMO from User1 to User2
    sys.DSL.Transfer(t,
        sys.Accounts.User1,
        sys.Accounts.User2.Address.Hex(),
        "30",
        sys.Manifest.DemoTokenAddr,
    )

    // Assert final balances
    require.Equal(t, "70", sys.DSL.GetBalance(t, sys.Accounts.User1, sys.Manifest.DemoTokenAddr))
    require.Equal(t, "30", sys.DSL.GetBalance(t, sys.Accounts.User2, sys.Manifest.DemoTokenAddr))
}

func TestTransfer_InsufficientBalance_Fails(t *testing.T) {
    sys := presets.NewAPIStack(t)

    sys.DSL.RegisterUser(t, sys.Accounts.User1)
    sys.DSL.RegisterUser(t, sys.Accounts.User2)
    // User1 has 0 DEMO

    _, err := sys.APIServer.Transfer(t.Context(), &stack.TransferRequest{
        From:      sys.Accounts.User1.Address,
        To:        sys.Accounts.User2.Address,
        Amount:    "100",
        TokenAddr: sys.Manifest.DemoTokenAddr,
    })
    require.Error(t, err)
}
```

### Bridge / Relayer Tests

```go
// tests/e2e/tests/bridge/main_test.go
package bridge_test

import (
    "testing"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

func TestMain(m *testing.M) {
    presets.DoMain(m)
}
```

```go
// tests/e2e/tests/bridge/deposit_test.go
package bridge_test

import (
    "math/big"
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

// TestDeposit_PROMPT_EthereumToCanton tests the full bridge deposit flow:
// User deposits ERC-20 on Ethereum → relayer picks it up → PROMPT appears on Canton.
func TestDeposit_PROMPT_EthereumToCanton(t *testing.T) {
    sys := presets.NewFullStack(t)

    // Setup: register user
    sys.DSL.RegisterUser(t, sys.Accounts.User1)
    sys.DSL.WaitForRelayerReady(t)

    // Verify initial Canton balance is 0
    require.Equal(t, "0", sys.DSL.GetBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr))

    // Verify user has PROMPT on Ethereum
    ethBal, err := sys.Anvil.ERC20Balance(
        t.Context(),
        common.HexToAddress(sys.Manifest.PromptTokenAddr),
        sys.Accounts.User1.Address,
    )
    require.NoError(t, err)
    require.True(t, ethBal.Cmp(big.NewInt(0)) > 0, "user1 should have initial PROMPT supply")

    // Action: deposit 100 PROMPT to Canton bridge
    depositAmount := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))
    sys.DSL.Deposit(t, sys.Accounts.User1, depositAmount)

    // Assert: wait for relayer to mint on Canton, check balance
    sys.DSL.WaitForCantonBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr, "100")
    require.Equal(t, "100", sys.DSL.GetBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr))
}

func TestDeposit_TwoUsers_Independent(t *testing.T) {
    sys := presets.NewFullStack(t)

    sys.DSL.RegisterUser(t, sys.Accounts.User1)
    sys.DSL.RegisterUser(t, sys.Accounts.User2)
    sys.DSL.WaitForRelayerReady(t)

    amount1 := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))
    amount2 := new(big.Int).Mul(big.NewInt(50), big.NewInt(1e18))

    sys.DSL.Deposit(t, sys.Accounts.User1, amount1)
    sys.DSL.Deposit(t, sys.Accounts.User2, amount2)

    sys.DSL.WaitForCantonBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr, "100")
    sys.DSL.WaitForCantonBalance(t, sys.Accounts.User2, sys.Manifest.PromptTokenAddr, "50")

    require.Equal(t, "100", sys.DSL.GetBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr))
    require.Equal(t, "50", sys.DSL.GetBalance(t, sys.Accounts.User2, sys.Manifest.PromptTokenAddr))
}
```

```go
// tests/e2e/tests/bridge/withdrawal_test.go
package bridge_test

import (
    "math/big"
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

// TestWithdrawal_PROMPT_CantonToEthereum tests the full withdrawal flow:
// User withdraws PROMPT from Canton → relayer processes → ERC-20 appears back on Ethereum.
func TestWithdrawal_PROMPT_CantonToEthereum(t *testing.T) {
    sys := presets.NewFullStack(t)

    // Setup: register user and deposit first
    sys.DSL.RegisterUser(t, sys.Accounts.User1)
    sys.DSL.WaitForRelayerReady(t)

    depositAmount := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))
    sys.DSL.Deposit(t, sys.Accounts.User1, depositAmount)
    sys.DSL.WaitForCantonBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr, "100")

    // Record Ethereum balance before withdrawal
    balBefore, _ := sys.Anvil.ERC20Balance(
        t.Context(),
        common.HexToAddress(sys.Manifest.PromptTokenAddr),
        sys.Accounts.User1.Address,
    )

    // Action: initiate withdrawal of 50 PROMPT back to Ethereum
    sys.DSL.Withdraw(t, sys.Accounts.User1, "50", sys.Manifest.PromptTokenAddr)

    // Assert: Canton balance decreases, Ethereum balance increases
    sys.DSL.WaitForCantonBalance(t, sys.Accounts.User1, sys.Manifest.PromptTokenAddr, "50")

    balAfter, _ := sys.Anvil.ERC20Balance(
        t.Context(),
        common.HexToAddress(sys.Manifest.PromptTokenAddr),
        sys.Accounts.User1.Address,
    )
    withdrawnAmount := new(big.Int).Mul(big.NewInt(50), big.NewInt(1e18))
    expected := new(big.Int).Add(balBefore, withdrawnAmount)
    require.Equal(t, expected, balAfter)
}
```

### Indexer Tests

```go
// tests/e2e/tests/indexer/main_test.go
package indexer_test

import (
    "testing"
    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
)

func TestMain(m *testing.M) {
    presets.DoMain(m)
}
```

```go
// tests/e2e/tests/indexer/mint_burn_test.go
//go:build e2e

package indexer_test

import (
    "math/big"
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

// TestIndexer_MintEvent_AfterDeposit verifies that a PROMPT deposit via the bridge
// produces a MINT event in the indexer with the correct party, amount, and bridge metadata.
func TestIndexer_MintEvent_AfterDeposit(t *testing.T) {
    sys := presets.NewFullStack(t)

    // Setup: register user and wait for relayer
    resp := sys.DSL.RegisterUser(t, sys.Accounts.User1)
    partyID := resp.CantonPartyID
    sys.DSL.WaitForRelayerReady(t)

    // Check initial state: indexer should have no balance yet
    initialSupply := sys.DSL.GetIndexerTotalSupply(t,
        sys.Manifest.PromptInstrumentAdmin,
        sys.Manifest.PromptInstrumentID,
    )

    // Action: deposit 100 PROMPT to Canton bridge
    depositAmount := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))
    sys.DSL.Deposit(t, sys.Accounts.User1, depositAmount)

    // Assert: wait for indexer to process the MINT event from the relayer
    sys.DSL.WaitForIndexerBalance(t, partyID,
        sys.Manifest.PromptInstrumentAdmin,
        sys.Manifest.PromptInstrumentID,
        "100",
    )

    // Verify balance via indexer
    bal := sys.DSL.GetIndexerBalance(t, partyID,
        sys.Manifest.PromptInstrumentAdmin,
        sys.Manifest.PromptInstrumentID,
    )
    require.Equal(t, "100", bal)

    // Verify total supply increased by 100
    newSupply := sys.DSL.GetIndexerTotalSupply(t,
        sys.Manifest.PromptInstrumentAdmin,
        sys.Manifest.PromptInstrumentID,
    )
    require.Equal(t, addDecimal(initialSupply, "100"), newSupply)

    // Verify MINT event was recorded with correct bridge metadata
    events := sys.DSL.ListIndexerPartyEvents(t, partyID, "MINT")
    require.NotEmpty(t, events, "at least one MINT event should be indexed")
    mint := events[len(events)-1]
    require.Equal(t, "MINT", mint.EventType)
    require.Equal(t, "100", mint.Amount)
    require.Nil(t, mint.FromPartyID)
    require.NotNil(t, mint.ToPartyID)
    require.Equal(t, partyID, *mint.ToPartyID)
    require.NotNil(t, mint.ExternalAddress, "bridge MINT should carry ExternalAddress metadata")
}

// TestIndexer_BurnEvent_AfterWithdrawal verifies that a withdrawal (Canton → Ethereum)
// produces a BURN event in the indexer and decrements the party's indexed balance.
func TestIndexer_BurnEvent_AfterWithdrawal(t *testing.T) {
    sys := presets.NewFullStack(t)

    resp := sys.DSL.RegisterUser(t, sys.Accounts.User1)
    partyID := resp.CantonPartyID
    sys.DSL.WaitForRelayerReady(t)

    // Setup: deposit first so there is a balance to burn
    depositAmount := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e18))
    sys.DSL.Deposit(t, sys.Accounts.User1, depositAmount)
    sys.DSL.WaitForIndexerBalance(t, partyID,
        sys.Manifest.PromptInstrumentAdmin, sys.Manifest.PromptInstrumentID, "100",
    )

    // Record supply before withdrawal
    supplyBefore := sys.DSL.GetIndexerTotalSupply(t,
        sys.Manifest.PromptInstrumentAdmin, sys.Manifest.PromptInstrumentID,
    )

    // Action: withdraw 50 PROMPT back to Ethereum
    sys.DSL.Withdraw(t, sys.Accounts.User1, "50", sys.Manifest.PromptTokenAddr)

    // Assert: indexer balance decreases to 50
    sys.DSL.WaitForIndexerBalance(t, partyID,
        sys.Manifest.PromptInstrumentAdmin, sys.Manifest.PromptInstrumentID, "50",
    )

    // Verify total supply decreased by 50
    supplyAfter := sys.DSL.GetIndexerTotalSupply(t,
        sys.Manifest.PromptInstrumentAdmin, sys.Manifest.PromptInstrumentID,
    )
    require.Equal(t, subtractDecimal(supplyBefore, "50"), supplyAfter)

    // Verify BURN event
    events := sys.DSL.ListIndexerPartyEvents(t, partyID, "BURN")
    require.NotEmpty(t, events)
    burn := events[len(events)-1]
    require.Equal(t, "BURN", burn.EventType)
    require.Equal(t, "50", burn.Amount)
    require.NotNil(t, burn.FromPartyID)
    require.Equal(t, partyID, *burn.FromPartyID)
    require.Nil(t, burn.ToPartyID)
}
```

```go
// tests/e2e/tests/indexer/transfer_test.go
//go:build e2e

package indexer_test

import (
    "testing"

    "github.com/chainsafe/canton-middleware/tests/e2e/devstack/presets"
    "github.com/stretchr/testify/require"
)

// TestIndexer_TransferEvent_AfterAPITransfer verifies that a DEMO token transfer via
// the api-server produces a TRANSFER event in the indexer for both sender and receiver.
func TestIndexer_TransferEvent_AfterAPITransfer(t *testing.T) {
    sys := presets.NewIndexerStack(t)

    // Setup: register both users and mint DEMO for user1
    resp1 := sys.DSL.RegisterUser(t, sys.Accounts.User1)
    resp2 := sys.DSL.RegisterUser(t, sys.Accounts.User2)
    partyID1 := resp1.CantonPartyID
    partyID2 := resp2.CantonPartyID

    sys.DSL.MintDEMO(t, sys.Accounts.User1, "100")
    sys.DSL.WaitForIndexerBalance(t, partyID1,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID, "100",
    )

    // Action: transfer 30 DEMO from user1 to user2
    sys.DSL.Transfer(t,
        sys.Accounts.User1,
        sys.Accounts.User2.Address.Hex(),
        "30",
        sys.Manifest.DemoTokenAddr,
    )

    // Assert: indexer reflects both sides of the transfer
    sys.DSL.WaitForIndexerBalance(t, partyID1,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID, "70",
    )
    sys.DSL.WaitForIndexerBalance(t, partyID2,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID, "30",
    )

    // Verify TRANSFER event recorded for sender
    events := sys.DSL.ListIndexerPartyEvents(t, partyID1, "TRANSFER")
    require.NotEmpty(t, events)
    xfer := events[len(events)-1]
    require.Equal(t, "TRANSFER", xfer.EventType)
    require.Equal(t, "30", xfer.Amount)
    require.NotNil(t, xfer.FromPartyID)
    require.Equal(t, partyID1, *xfer.FromPartyID)
    require.NotNil(t, xfer.ToPartyID)
    require.Equal(t, partyID2, *xfer.ToPartyID)

    // Total supply should not change on TRANSFER (no mint/burn)
    supply := sys.DSL.GetIndexerTotalSupply(t,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID,
    )
    require.Equal(t, "100", supply)
}

// TestIndexer_HolderCount_Updates verifies that the holder count increments when a new
// party receives tokens and decrements when a party's balance reaches zero.
func TestIndexer_HolderCount_Updates(t *testing.T) {
    sys := presets.NewIndexerStack(t)

    resp1 := sys.DSL.RegisterUser(t, sys.Accounts.User1)
    resp2 := sys.DSL.RegisterUser(t, sys.Accounts.User2)
    partyID1 := resp1.CantonPartyID
    partyID2 := resp2.CantonPartyID

    sys.DSL.MintDEMO(t, sys.Accounts.User1, "50")
    sys.DSL.WaitForIndexerBalance(t, partyID1,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID, "50",
    )

    token, err := sys.Indexer.GetToken(t.Context(),
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID,
    )
    require.NoError(t, err)
    require.Equal(t, int64(1), token.HolderCount)

    // Transfer all to user2 — user1 balance becomes 0, holder count stays 1
    sys.DSL.Transfer(t,
        sys.Accounts.User1, sys.Accounts.User2.Address.Hex(), "50", sys.Manifest.DemoTokenAddr,
    )
    sys.DSL.WaitForIndexerBalance(t, partyID2,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID, "50",
    )
    sys.DSL.WaitForIndexerBalance(t, partyID1,
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID, "0",
    )

    token, err = sys.Indexer.GetToken(t.Context(),
        sys.Manifest.DemoInstrumentAdmin, sys.Manifest.DemoInstrumentID,
    )
    require.NoError(t, err)
    require.Equal(t, int64(1), token.HolderCount, "holder count unchanged: user1 left, user2 arrived")
    _ = partyID1
    _ = partyID2
}
```

---

## 11. Full Directory Layout

```
tests/e2e/
│
├── devstack/                           ← framework (analogous to op-devstack)
│   │
│   ├── stack/                          Layer 1: Interfaces + shared types
│   │   ├── interfaces.go               Anvil, Canton, APIServer, Relayer, Indexer, Postgres
│   │   └── types.go                    ServiceManifest, Account, RegisterRequest/Response,
│   │                                   TransferRequest, UserRow, AnvilAccount0/1,
│   │                                   IndexerToken, IndexerBalance, IndexerEvent,
│   │                                   IndexerTokenPage, IndexerBalancePage, IndexerEventPage
│   │
│   ├── shim/                           Layer 2: Concrete service clients
│   │   ├── anvil.go                    go-ethereum ethclient + ABI calls
│   │   ├── canton.go                   Canton gRPC health check wrapper
│   │   ├── apiserver.go                HTTP client for /register, /eth JSON-RPC
│   │   ├── relayer.go                  HTTP client for /health, /ready
│   │   ├── indexer.go                  HTTP client for /indexer/v1/admin/* (getJSON helper)
│   │   └── postgres.go                 database/sql client, whitelist + user queries
│   │
│   ├── docker/                         Layer 3: Orchestration + discovery
│   │   ├── compose.go                  ComposeOrchestrator: Start/Stop via docker CLI
│   │   └── discovery.go                ServiceDiscovery: reads published ports, deployer output
│   │
│   ├── system/                         Layer 4: System composition
│   │   └── system.go                   System{Anvil, Canton, APIServer, Relayer, Indexer, Postgres, DSL}
│   │                                   New(t, manifest) wires all shims
│   │
│   ├── dsl/                            Layer 5: High-level test helpers
│   │   ├── dsl.go                      DSL{RegisterUser, Deposit, Withdraw, Transfer,
│   │   │                               GetBalance, WaitForCantonBalance, WaitForRelayerReady,
│   │   │                               GetIndexerBalance, WaitForIndexerBalance,
│   │   │                               GetIndexerTotalSupply, WaitForIndexerTotalSupply,
│   │   │                               GetIndexerEvent, WaitForIndexerEvent,
│   │   │                               ListIndexerPartyEvents}
│   │   └── helpers.go                  signEIP191, balanceEquals, addDecimal, subtractDecimal,
│   │                                   toWei, fromWei
│   │
│   └── presets/                        Layer 6: Test entry points
│       ├── presets.go                  DoMain(m, opts), NewFullStack(t), NewAPIStack(t),
│       │                               NewIndexerStack(t)
│       └── options.go                  WithComposeFile, WithProjectName, envOr helper
│
├── tests/
│   │
│   ├── api/                            api-server tests
│   │   ├── main_test.go                func TestMain(m) { presets.DoMain(m) }
│   │   ├── register_test.go            TestRegister_NewUser_Success
│   │   │                               TestRegister_Duplicate_Idempotent
│   │   │                               TestRegister_NotWhitelisted_Fails
│   │   ├── balance_test.go             TestGetBalance_UnregisteredUser_ReturnsZero
│   │   │                               TestGetBalance_AfterMintDEMO
│   │   │                               TestGetTokenMetadata (name/symbol/decimals/totalSupply)
│   │   └── transfer_test.go            TestTransfer_DEMO_BetweenUsers
│   │                                   TestTransfer_InsufficientBalance_Fails
│   │                                   TestTransfer_PROMPT_AfterDeposit
│   │
│   ├── bridge/                         relayer + bridge tests
│   │   ├── main_test.go                func TestMain(m) { presets.DoMain(m) }
│   │   ├── deposit_test.go             TestDeposit_PROMPT_EthereumToCanton
│   │   │                               TestDeposit_TwoUsers_Independent
│   │   │                               TestDeposit_SmallAmount
│   │   └── withdrawal_test.go          TestWithdrawal_PROMPT_CantonToEthereum
│   │                                   TestWithdrawal_PartialAmount
│   │                                   TestWithdrawal_AfterTransfer
│   │
│   └── indexer/                        indexer event indexing + balance tests
│       ├── main_test.go                func TestMain(m) { presets.DoMain(m) }
│       ├── mint_burn_test.go           TestIndexer_MintEvent_AfterDeposit
│       │                               TestIndexer_BurnEvent_AfterWithdrawal
│       └── transfer_test.go            TestIndexer_TransferEvent_AfterAPITransfer
│                                       TestIndexer_HolderCount_Updates
│
├── docker-compose.e2e.yaml             ← test-specific compose (see §12)
└── README.md
```

---

## 12. Docker Compose & Service Discovery

### `tests/e2e/docker-compose.e2e.yaml`

The e2e compose file is a thin wrapper around the main compose. It adds explicit
`healthcheck` definitions and publishes ports to known host ports so service discovery
is deterministic (no random ephemeral ports).

```yaml
# tests/e2e/docker-compose.e2e.yaml
# Extends the root docker-compose.yaml with e2e-specific settings.
# Key differences:
#   - All ports explicitly mapped to localhost
#   - Bootstrap container writes /tmp/e2e-deploy.json
#   - Services have stricter healthchecks

include:
  - path: ../../docker-compose.yaml
    # Override only what we need for testing

services:
  anvil:
    ports:
      - "8545:8545"
    healthcheck:
      test: ["CMD", "cast", "block-number", "--rpc-url", "http://localhost:8545"]
      interval: 2s
      timeout: 5s
      retries: 30

  canton:
    ports:
      - "5011:5011"   # gRPC Ledger API
      - "5013:5013"   # HTTP REST API
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:5013/health"]
      interval: 5s
      timeout: 10s
      retries: 30
      start_period: 30s

  postgres:
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "postgres"]
      interval: 2s
      retries: 20

  oauth2-mock:
    ports:
      - "8088:8088"

  bootstrap:
    # Existing bootstrap container — runs bootstrap-bridge.go + canton scripts
    # ADDED: writes contract addresses to a shared volume file
    volumes:
      - e2e-deploy:/tmp
    command: >
      sh -c "
        /bootstrap/run.sh &&
        echo '{\"prompt_token\":\"'$$PROMPT_TOKEN_ADDRESS'\",\"canton_bridge\":\"'$$BRIDGE_ADDRESS'\"}' > /tmp/e2e-deploy.json
      "
    depends_on:
      anvil:
        condition: service_healthy
      canton:
        condition: service_healthy
      postgres:
        condition: service_healthy

  api-server:
    ports:
      - "8081:8081"
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:8081/health"]
      interval: 3s
      retries: 20
    depends_on:
      bootstrap:
        condition: service_completed_successfully

  relayer:
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:8080/health"]
      interval: 3s
      retries: 20
    depends_on:
      bootstrap:
        condition: service_completed_successfully

  indexer:
    ports:
      - "8082:8082"
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:8082/health"]
      interval: 3s
      retries: 20
    depends_on:
      bootstrap:
        condition: service_completed_successfully
      postgres:
        condition: service_healthy

volumes:
  e2e-deploy:
```

### Service Discovery Flow

```
TestMain calls presets.DoMain(m)
    │
    ▼
ComposeOrchestrator.Start()
    │   docker compose -f docker-compose.e2e.yaml -p canton-e2e up --build --wait
    │   (--wait: blocks until ALL healthchecks pass)
    ▼
ServiceDiscovery.Manifest()
    │   docker compose -p canton-e2e port anvil 8545       → "0.0.0.0:8545"
    │   docker compose -p canton-e2e port canton 5011      → "0.0.0.0:5011"
    │   docker compose -p canton-e2e port api-server 8081  → "0.0.0.0:8081"
    │   docker compose -p canton-e2e port relayer 8080     → "0.0.0.0:8080"
    │   docker compose -p canton-e2e port indexer 8082     → "0.0.0.0:8082"
    │   ... (all services)
    │   reads /tmp/e2e-deploy.json from bootstrap volume for contract addresses
    │   and Canton instrument admin party IDs
    ▼
ServiceManifest{
    AnvilRPC:     "http://localhost:8545",
    CantonGRPC:   "localhost:5011",
    CantonHTTP:   "http://localhost:5013",
    APIHTTP:      "http://localhost:8081",
    RelayerHTTP:  "http://localhost:8080",
    IndexerHTTP:  "http://localhost:8082",
    PostgresDSN:  "postgres://postgres:p@ssw0rd@localhost:5432/erc20_api",
    PromptTokenAddr:       "0x5FbDB...",
    BridgeAddr:            "0xe7f17...",
    PromptInstrumentAdmin: "PromptAdmin::...",  // Canton party ID
    PromptInstrumentID:    "PROMPT",
    DemoInstrumentAdmin:   "DemoAdmin::...",
    DemoInstrumentID:      "DEMO",
}
    │
    ▼
system.New(t, manifest)
    │
    ▼  each test calls:
presets.NewFullStack(t) → *system.System  (shares manifest, fresh shim instances)
```

---

## 13. Configuration & Environment Variables

| Variable              | Default                              | Purpose                                                            |
|-----------------------|--------------------------------------|--------------------------------------------------------------------|
| `E2E_COMPOSE_FILE`    | `tests/e2e/docker-compose.e2e.yaml`  | Path to compose file                                               |
| `E2E_PROJECT_NAME`    | `canton-e2e`                         | Docker compose project name                                        |
| `E2E_KEEP_SERVICES`   | `false`                              | If `true`, do not `docker compose down` after tests                |
| `E2E_TIMEOUT`         | `30s`                                | Default WaitFor* timeout (indexer helpers use 60s for stream lag)  |

### Running the tests

```bash
# Run all e2e tests (starts docker, runs tests, tears down)
go test ./tests/e2e/tests/... -v -timeout 5m -tags e2e

# Run only api-server tests
go test ./tests/e2e/tests/api/... -v -timeout 3m -tags e2e

# Run only bridge tests
go test ./tests/e2e/tests/bridge/... -v -timeout 5m -tags e2e

# Run only indexer tests
go test ./tests/e2e/tests/indexer/... -v -timeout 5m -tags e2e

# Keep services running after tests (useful for debugging)
E2E_KEEP_SERVICES=true go test ./tests/e2e/tests/... -v -tags e2e

# Use an already-running stack (skips docker up/down)
E2E_KEEP_SERVICES=true E2E_COMPOSE_FILE=/dev/null go test ./tests/e2e/tests/api/...

# Run a single test
go test ./tests/e2e/tests/bridge/... -run TestDeposit_PROMPT_EthereumToCanton -v -tags e2e
go test ./tests/e2e/tests/indexer/... -run TestIndexer_MintEvent_AfterDeposit -v -tags e2e
```

### Build tag

All e2e test files carry `//go:build e2e` to prevent them running in `go test ./...`:

```go
//go:build e2e

package api_test
```

### Makefile targets

```makefile
.PHONY: test-e2e test-e2e-api test-e2e-bridge test-e2e-indexer test-e2e-clean

test-e2e:
	go test ./tests/e2e/tests/... -v -timeout 5m -tags e2e

test-e2e-api:
	go test ./tests/e2e/tests/api/... -v -timeout 3m -tags e2e

test-e2e-bridge:
	go test ./tests/e2e/tests/bridge/... -v -timeout 5m -tags e2e

test-e2e-indexer:
	go test ./tests/e2e/tests/indexer/... -v -timeout 5m -tags e2e

test-e2e-clean:
	docker compose -f tests/e2e/docker-compose.e2e.yaml -p canton-e2e down -v --remove-orphans
```

---

