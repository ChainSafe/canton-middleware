package relayer

// TODO: remove the mock impl and use mockery to generate mock

import (
	"context"
	"math/big"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/db"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// MockCantonClient is a mock implementation of CantonBridgeClient
type MockCantonClient struct {
	// Issuer-centric model methods
	StreamWithdrawalEventsFunc func(ctx context.Context, offset string) <-chan *canton.WithdrawalEvent
	CreatePendingDepositFunc   func(ctx context.Context, req canton.CreatePendingDepositRequest) (*canton.PendingDeposit, error)
	ProcessDepositAndMintFunc  func(ctx context.Context, req canton.ProcessDepositRequest) (*canton.ProcessedDeposit, error)
	IsDepositProcessedFunc     func(ctx context.Context, evmTxHash string) (bool, error)
	InitiateWithdrawalFunc     func(ctx context.Context, req canton.InitiateWithdrawalRequest) (string, error)
	CompleteWithdrawalFunc     func(ctx context.Context, req canton.CompleteWithdrawalRequest) error
	GetLatestLedgerOffsetFunc  func(ctx context.Context) (int64, error)
}

func (m *MockCantonClient) StreamWithdrawalEvents(ctx context.Context, offset string) <-chan *canton.WithdrawalEvent {
	if m.StreamWithdrawalEventsFunc != nil {
		return m.StreamWithdrawalEventsFunc(ctx, offset)
	}
	return nil
}

func (m *MockCantonClient) CreatePendingDeposit(
	ctx context.Context,
	req canton.CreatePendingDepositRequest) (*canton.PendingDeposit, error) {
	if m.CreatePendingDepositFunc != nil {
		return m.CreatePendingDepositFunc(ctx, req)
	}
	return nil, nil
}

func (m *MockCantonClient) ProcessDepositAndMint(
	ctx context.Context,
	req canton.ProcessDepositRequest) (*canton.ProcessedDeposit, error) {
	if m.ProcessDepositAndMintFunc != nil {
		return m.ProcessDepositAndMintFunc(ctx, req)
	}
	return nil, nil
}

func (m *MockCantonClient) IsDepositProcessed(ctx context.Context, evmTxHash string) (bool, error) {
	if m.IsDepositProcessedFunc != nil {
		return m.IsDepositProcessedFunc(ctx, evmTxHash)
	}
	return false, nil
}

func (m *MockCantonClient) InitiateWithdrawal(ctx context.Context, req canton.InitiateWithdrawalRequest) (string, error) {
	if m.InitiateWithdrawalFunc != nil {
		return m.InitiateWithdrawalFunc(ctx, req)
	}
	return "", nil
}

func (m *MockCantonClient) CompleteWithdrawal(ctx context.Context, req canton.CompleteWithdrawalRequest) error {
	if m.CompleteWithdrawalFunc != nil {
		return m.CompleteWithdrawalFunc(ctx, req)
	}
	return nil
}

func (m *MockCantonClient) GetLatestLedgerOffset(ctx context.Context) (int64, error) {
	if m.GetLatestLedgerOffsetFunc != nil {
		return m.GetLatestLedgerOffsetFunc(ctx)
	}
	return 0, nil
}

func (m *MockCantonClient) GetWayfinderBridgeConfigCID(ctx context.Context) (string, error) {
	return m.GetWayfinderBridgeConfigCID(ctx)
}

// MockEthereumClient is a mock implementation of EthereumBridgeClient
type MockEthereumClient struct {
	GetLatestBlockNumberFunc  func(ctx context.Context) (uint64, error)
	WithdrawFromCantonFunc    func(ctx context.Context, token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (common.Hash, error)
	WatchDepositEventsFunc    func(ctx context.Context, fromBlock uint64, handler func(*ethereum.DepositEvent) error) error
	IsWithdrawalProcessedFunc func(ctx context.Context, cantonTxHash [32]byte) (bool, error)
	LastScannedBlock          uint64
}

func (m *MockEthereumClient) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	if m.GetLatestBlockNumberFunc != nil {
		return m.GetLatestBlockNumberFunc(ctx)
	}
	return 0, nil
}

func (m *MockEthereumClient) WithdrawFromCanton(ctx context.Context, token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (common.Hash, error) {
	if m.WithdrawFromCantonFunc != nil {
		return m.WithdrawFromCantonFunc(ctx, token, recipient, amount, nonce, cantonTxHash)
	}
	return common.Hash{}, nil
}

func (m *MockEthereumClient) WatchDepositEvents(ctx context.Context, fromBlock uint64, handler func(*ethereum.DepositEvent) error) error {
	if m.WatchDepositEventsFunc != nil {
		return m.WatchDepositEventsFunc(ctx, fromBlock, handler)
	}
	return nil
}

func (m *MockEthereumClient) IsWithdrawalProcessed(ctx context.Context, cantonTxHash [32]byte) (bool, error) {
	if m.IsWithdrawalProcessedFunc != nil {
		return m.IsWithdrawalProcessedFunc(ctx, cantonTxHash)
	}
	return false, nil
}

func (m *MockEthereumClient) GetLastScannedBlock() uint64 {
	return m.LastScannedBlock
}

// MockStore is a mock implementation of BridgeStore
type MockStore struct {
	GetTransferFunc          func(id string) (*db.Transfer, error)
	CreateTransferFunc       func(transfer *db.Transfer) error
	UpdateTransferStatusFunc func(id string, status db.TransferStatus, destTxHash *string) error
	GetChainStateFunc        func(chainID string) (*db.ChainState, error)
	SetChainStateFunc        func(chainID string, blockNumber int64, blockHash string) error
	GetPendingTransfersFunc  func(direction db.TransferDirection) ([]*db.Transfer, error)
	ListTransfersFunc        func(limit int) ([]*db.Transfer, error)
}

func (m *MockStore) GetTransfer(id string) (*db.Transfer, error) {
	if m.GetTransferFunc != nil {
		return m.GetTransferFunc(id)
	}
	return nil, nil
}

func (m *MockStore) CreateTransfer(transfer *db.Transfer) error {
	if m.CreateTransferFunc != nil {
		return m.CreateTransferFunc(transfer)
	}
	return nil
}

func (m *MockStore) UpdateTransferStatus(id string, status db.TransferStatus, destTxHash *string) error {
	if m.UpdateTransferStatusFunc != nil {
		return m.UpdateTransferStatusFunc(id, status, destTxHash)
	}
	return nil
}

func (m *MockStore) GetChainState(chainID string) (*db.ChainState, error) {
	if m.GetChainStateFunc != nil {
		return m.GetChainStateFunc(chainID)
	}
	return nil, nil
}

func (m *MockStore) SetChainState(chainID string, blockNumber int64, blockHash string) error {
	if m.SetChainStateFunc != nil {
		return m.SetChainStateFunc(chainID, blockNumber, blockHash)
	}
	return nil
}

func (m *MockStore) GetPendingTransfers(direction db.TransferDirection) ([]*db.Transfer, error) {
	if m.GetPendingTransfersFunc != nil {
		return m.GetPendingTransfersFunc(direction)
	}
	return nil, nil
}

func (m *MockStore) ListTransfers(limit int) ([]*db.Transfer, error) {
	if m.ListTransfersFunc != nil {
		return m.ListTransfersFunc(limit)
	}
	return nil, nil
}
