package relayer

// Canton and Ethereum client mocks live here for use in package-level tests.
// These are separate from the BridgeStore mock (which is in pkg/relayer/mocks/)
// because they satisfy external interfaces (canton.Bridge, EthereumBridgeClient).

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
)

// MockCantonClient is a test double for canton.Bridge.
type MockCantonClient struct {
	StreamWithdrawalEventsFunc func(ctx context.Context, offset string) <-chan *canton.WithdrawalEvent
	CreatePendingDepositFunc   func(ctx context.Context, req canton.CreatePendingDepositRequest) (*canton.PendingDeposit, error)
	ProcessDepositAndMintFunc  func(ctx context.Context, req canton.ProcessDepositRequest) (*canton.ProcessedDeposit, error)
	IsDepositProcessedFunc     func(ctx context.Context, evmTxHash string) (bool, error)
	InitiateWithdrawalFunc     func(ctx context.Context, req canton.InitiateWithdrawalRequest) (string, error)
	CompleteWithdrawalFunc     func(ctx context.Context, req canton.CompleteWithdrawalRequest) error
	GetLatestLedgerOffsetFunc  func(ctx context.Context) (int64, error)
	GetBridgeConfigCIDFunc     func(ctx context.Context) (string, error)
}

func (m *MockCantonClient) StreamWithdrawalEvents(ctx context.Context, offset string) <-chan *canton.WithdrawalEvent {
	if m.StreamWithdrawalEventsFunc != nil {
		return m.StreamWithdrawalEventsFunc(ctx, offset)
	}
	ch := make(chan *canton.WithdrawalEvent)
	close(ch)
	return ch
}

func (m *MockCantonClient) CreatePendingDeposit(ctx context.Context, req canton.CreatePendingDepositRequest) (*canton.PendingDeposit, error) {
	if m.CreatePendingDepositFunc != nil {
		return m.CreatePendingDepositFunc(ctx, req)
	}
	return nil, nil
}

func (m *MockCantonClient) ProcessDepositAndMint(ctx context.Context, req canton.ProcessDepositRequest) (*canton.ProcessedDeposit, error) {
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
	if m.GetBridgeConfigCIDFunc != nil {
		return m.GetBridgeConfigCIDFunc(ctx)
	}
	return "", nil
}

// MockEthereumClient is a test double for EthereumBridgeClient.
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

func (m *MockEthereumClient) WithdrawFromCanton(ctx context.Context, tkn common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (common.Hash, error) {
	if m.WithdrawFromCantonFunc != nil {
		return m.WithdrawFromCantonFunc(ctx, tkn, recipient, amount, nonce, cantonTxHash)
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

// MockStore is a test double for BridgeStore.
type MockStore struct {
	GetTransferFunc          func(ctx context.Context, id string) (*Transfer, error)
	CreateTransferFunc       func(ctx context.Context, transfer *Transfer) (bool, error)
	UpdateTransferStatusFunc func(ctx context.Context, id string, status TransferStatus, destTxHash *string, errMsg *string) error
	IncrementRetryCountFunc  func(ctx context.Context, id string) error
	GetChainStateFunc        func(ctx context.Context, chainID string) (*ChainState, error)
	SetChainStateFunc        func(ctx context.Context, chainID string, blockNumber int64, offset string) error
	GetPendingTransfersFunc  func(ctx context.Context, direction TransferDirection) ([]*Transfer, error)
	ListTransfersFunc        func(ctx context.Context, limit int) ([]*Transfer, error)
}

func (m *MockStore) GetTransfer(ctx context.Context, id string) (*Transfer, error) {
	if m.GetTransferFunc != nil {
		return m.GetTransferFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockStore) CreateTransfer(ctx context.Context, transfer *Transfer) (bool, error) {
	if m.CreateTransferFunc != nil {
		return m.CreateTransferFunc(ctx, transfer)
	}
	return true, nil
}

func (m *MockStore) UpdateTransferStatus(ctx context.Context, id string, status TransferStatus, destTxHash *string, errMsg *string) error {
	if m.UpdateTransferStatusFunc != nil {
		return m.UpdateTransferStatusFunc(ctx, id, status, destTxHash, errMsg)
	}
	return nil
}

func (m *MockStore) IncrementRetryCount(ctx context.Context, id string) error {
	if m.IncrementRetryCountFunc != nil {
		return m.IncrementRetryCountFunc(ctx, id)
	}
	return nil
}

func (m *MockStore) GetChainState(ctx context.Context, chainID string) (*ChainState, error) {
	if m.GetChainStateFunc != nil {
		return m.GetChainStateFunc(ctx, chainID)
	}
	return nil, nil
}

func (m *MockStore) SetChainState(ctx context.Context, chainID string, blockNumber int64, offset string) error {
	if m.SetChainStateFunc != nil {
		return m.SetChainStateFunc(ctx, chainID, blockNumber, offset)
	}
	return nil
}

func (m *MockStore) GetPendingTransfers(ctx context.Context, direction TransferDirection) ([]*Transfer, error) {
	if m.GetPendingTransfersFunc != nil {
		return m.GetPendingTransfersFunc(ctx, direction)
	}
	return nil, nil
}

func (m *MockStore) ListTransfers(ctx context.Context, limit int) ([]*Transfer, error) {
	if m.ListTransfersFunc != nil {
		return m.ListTransfersFunc(ctx, limit)
	}
	return nil, nil
}
