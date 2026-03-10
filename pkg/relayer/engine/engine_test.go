package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	relayermocks "github.com/chainsafe/canton-middleware/pkg/relayer/engine/mocks"
)

func newReconciliationEngine(cfg *config.Config, store BridgeStore) *Engine {
	return &Engine{
		config: cfg,
		store:  store,
		logger: zap.NewNop(),
	}
}

func TestEngine_IsReady_InitiallyFalse(t *testing.T) {
	engine := NewEngine(&config.Config{}, nil, nil, nil, zap.NewNop())
	if engine.IsReady() {
		t.Fatalf("engine should not be ready initially")
	}
}

func TestEngine_Start_ReturnsLoadOffsetError(t *testing.T) {
	ctx := context.Background()
	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetChainState(mock.Anything, relayer.ChainCanton).Return(nil, errors.New("db down")).Once()

	engine := NewEngine(&config.Config{}, relayermocks.NewCantonBridge(t), relayermocks.NewEthereumBridgeClient(t), store, zap.NewNop())
	err := engine.Start(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to load offsets") {
		t.Fatalf("expected load offsets error, got %v", err)
	}
}

func TestEngine_StartAndStop_WithMockedDependencies(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetChainState(mock.Anything, relayer.ChainCanton).
		Return(&relayer.ChainState{ChainID: relayer.ChainCanton, Offset: "10"}, nil).Once()
	store.EXPECT().GetChainState(mock.Anything, relayer.ChainEthereum).
		Return(&relayer.ChainState{ChainID: relayer.ChainEthereum, LastBlock: 20}, nil).Once()

	cantonClient := relayermocks.NewCantonBridge(t)
	cantonEvents := make(chan *canton.WithdrawalEvent)
	close(cantonEvents)
	cantonClient.EXPECT().StreamWithdrawalEvents(mock.Anything, "10").Return((<-chan *canton.WithdrawalEvent)(cantonEvents)).Maybe()
	cantonClient.EXPECT().GetLatestLedgerOffset(mock.Anything).Return(int64(100), nil).Maybe()

	ethClient := relayermocks.NewEthereumBridgeClient(t)
	ethClient.EXPECT().WatchDepositEvents(mock.Anything, uint64(20), mock.Anything).Return(nil).Maybe()
	ethClient.EXPECT().GetLatestBlockNumber(mock.Anything).Return(uint64(20), nil).Maybe()
	ethClient.EXPECT().GetLastScannedBlock().Return(uint64(20)).Maybe()

	engine := NewEngine(cfg, cantonClient, ethClient, store, zap.NewNop())
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	engine.Stop()
	if engine.IsReady() {
		t.Fatalf("engine should not be marked ready immediately")
	}
}

func TestEngine_RunReconciliation_CantonPendingQueryError(t *testing.T) {
	ctx := context.Background()
	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return(nil, errors.New("boom")).Once()

	engine := newReconciliationEngine(&config.Config{}, store)
	err := engine.runReconciliation(ctx)
	if err == nil || !strings.Contains(err.Error(), "get pending canton transfers") {
		t.Fatalf("expected canton pending query error, got %v", err)
	}
}

func TestEngine_RunReconciliation_EthereumPendingQueryError(t *testing.T) {
	ctx := context.Background()
	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return(nil, errors.New("boom")).Once()

	engine := newReconciliationEngine(&config.Config{}, store)
	err := engine.runReconciliation(ctx)
	if err == nil || !strings.Contains(err.Error(), "get pending ethereum transfers") {
		t.Fatalf("expected ethereum pending query error, got %v", err)
	}
}

func TestEngine_RunReconciliation_MaxRetriesMarksFailed(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.MaxRetries = 2
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{ID: "t1", RetryCount: 2, UpdatedAt: time.Now().Add(-2 * time.Second)}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().UpdateTransferStatus(
		ctx,
		"t1",
		relayer.TransferStatusFailed,
		(*string)(nil),
		mock.MatchedBy(func(v *string) bool { return v != nil && strings.Contains(*v, "max retries (2) exceeded") }),
	).Return(nil).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = relayermocks.NewDestination(t)
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() failed: %v", err)
	}
}

func TestEngine_RunReconciliation_MaxRetriesUpdateErrorIgnored(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.MaxRetries = 1
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{ID: "t1", RetryCount: 1, UpdatedAt: time.Now().Add(-2 * time.Second)}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().UpdateTransferStatus(mock.Anything, "t1", relayer.TransferStatusFailed, (*string)(nil), mock.Anything).
		Return(errors.New("db write failed")).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = relayermocks.NewDestination(t)
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() should ignore update failure, got %v", err)
	}
}

func TestEngine_RunReconciliation_BeforeRetryDelayDoesNothing(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.MaxRetries = 3
	cfg.Bridge.RetryDelay = time.Hour

	transfer := &relayer.Transfer{ID: "t1", UpdatedAt: time.Now()}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()

	dest := relayermocks.NewDestination(t)
	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = dest
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() failed: %v", err)
	}
}

func TestEngine_RunReconciliation_RetrySuccessUpdatesCompletedWithHash(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{
		ID:                "t1",
		Direction:         relayer.DirectionCantonToEthereum,
		SourceChain:       relayer.ChainCanton,
		DestinationChain:  relayer.ChainEthereum,
		SourceTxHash:      "0xsource",
		TokenAddress:      "0xtoken",
		Amount:            "10",
		Sender:            "alice",
		Recipient:         "bob",
		Nonce:             1,
		SourceBlockNumber: 5,
		UpdatedAt:         time.Now().Add(-2 * time.Second),
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().UpdateTransferStatus(
		ctx,
		"t1",
		relayer.TransferStatusCompleted,
		mock.MatchedBy(func(v *string) bool { return v != nil && *v == "0xhash" }),
		(*string)(nil),
	).Return(nil).Once()

	dest := relayermocks.NewDestination(t)
	dest.EXPECT().SubmitTransfer(ctx, mock.MatchedBy(func(event *relayer.Event) bool {
		return event != nil && event.ID == "t1" && event.SourceChain == relayer.ChainCanton && event.DestinationChain == relayer.ChainEthereum
	})).Return("0xhash", false, nil).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = dest
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() failed: %v", err)
	}
}

func TestEngine_RunReconciliation_RetrySuccessUpdateErrorIgnored(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{ID: "t1", UpdatedAt: time.Now().Add(-2 * time.Second), CreatedAt: time.Now().Add(-1 * time.Minute)}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().UpdateTransferStatus(mock.Anything, "t1", relayer.TransferStatusCompleted, mock.Anything, (*string)(nil)).
		Return(errors.New("db write failed")).Once()

	dest := relayermocks.NewDestination(t)
	dest.EXPECT().SubmitTransfer(ctx, mock.AnythingOfType("*relayer.Event")).Return("0xhash", false, nil).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = dest
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() should ignore update failure, got %v", err)
	}
}

func TestEngine_RunReconciliation_RetrySkippedUpdatesCompletedWithoutHash(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{
		ID:                "t1",
		Direction:         relayer.DirectionEthereumToCanton,
		SourceChain:       relayer.ChainEthereum,
		DestinationChain:  relayer.ChainCanton,
		SourceTxHash:      "0xsource",
		TokenAddress:      "0xtoken",
		Amount:            "10",
		Sender:            "alice",
		Recipient:         "bob",
		Nonce:             1,
		SourceBlockNumber: 5,
		UpdatedAt:         time.Now().Add(-2 * time.Second),
		CreatedAt:         time.Now().Add(-1 * time.Minute),
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().UpdateTransferStatus(ctx, "t1", relayer.TransferStatusCompleted, (*string)(nil), (*string)(nil)).Return(nil).Once()

	dest := relayermocks.NewDestination(t)
	dest.EXPECT().SubmitTransfer(ctx, mock.AnythingOfType("*relayer.Event")).Return("", true, nil).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = nil
	engine.cantonDest = dest

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() failed: %v", err)
	}
}

func TestEngine_RunReconciliation_RetryFailureIncrementsRetryCount(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{ID: "t1", UpdatedAt: time.Now().Add(-2 * time.Second), CreatedAt: time.Now().Add(-1 * time.Minute)}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().IncrementRetryCount(ctx, "t1").Return(nil).Once()

	dest := relayermocks.NewDestination(t)
	dest.EXPECT().SubmitTransfer(ctx, mock.AnythingOfType("*relayer.Event")).Return("", false, errors.New("submit failed")).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = dest
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() failed: %v", err)
	}
}

func TestEngine_RunReconciliation_RetryFailureIncrementErrorIgnored(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{ID: "t1", UpdatedAt: time.Now().Add(-2 * time.Second), CreatedAt: time.Now().Add(-1 * time.Minute)}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()
	store.EXPECT().IncrementRetryCount(ctx, "t1").Return(errors.New("db write failed")).Once()

	dest := relayermocks.NewDestination(t)
	dest.EXPECT().SubmitTransfer(ctx, mock.AnythingOfType("*relayer.Event")).Return("", false, errors.New("submit failed")).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = dest
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() should ignore increment failure, got %v", err)
	}
}

func TestEngine_RunReconciliation_NilDestinationIsNoOp(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Bridge.RetryDelay = time.Second

	transfer := &relayer.Transfer{ID: "t1", UpdatedAt: time.Now().Add(-2 * time.Second), CreatedAt: time.Now().Add(-1 * time.Minute)}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum).Return([]*relayer.Transfer{transfer}, nil).Once()
	store.EXPECT().GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton).Return([]*relayer.Transfer{}, nil).Once()

	engine := newReconciliationEngine(cfg, store)
	engine.ethDest = nil
	engine.cantonDest = nil

	if err := engine.runReconciliation(ctx); err != nil {
		t.Fatalf("runReconciliation() failed: %v", err)
	}
}
