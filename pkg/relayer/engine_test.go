package relayer_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	bridgesdk "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	bridgemocks "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge/mocks"
	"github.com/chainsafe/canton-middleware/pkg/config"
	relayer "github.com/chainsafe/canton-middleware/pkg/relayer"
	relayermocks "github.com/chainsafe/canton-middleware/pkg/relayer/mocks"
)

func TestEngine_IsReady_InitiallyFalse(t *testing.T) {
	engine := relayer.NewEngine(&config.Config{}, nil, nil, nil, zap.NewNop())
	if engine.IsReady() {
		t.Fatalf("engine should not be ready initially")
	}
}

func TestEngine_Start_ReturnsLoadOffsetError(t *testing.T) {
	ctx := context.Background()
	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetChainState(mock.Anything, relayer.ChainCanton).Return(nil, errors.New("db down")).Once()

	engine := relayer.NewEngine(&config.Config{}, bridgemocks.NewBridgeMock(t), relayermocks.NewEthereumBridgeClient(t), store, zap.NewNop())
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

	cantonClient := bridgemocks.NewBridgeMock(t)
	cantonClient.EXPECT().GetLatestLedgerOffset(mock.Anything).Return(int64(100), nil).Once()
	cantonClient.EXPECT().StreamWithdrawalEvents(mock.Anything, "10").
		RunAndReturn(func(_ context.Context, _ string) <-chan *bridgesdk.WithdrawalEvent {
			ch := make(chan *bridgesdk.WithdrawalEvent)
			close(ch)
			return ch
		}).Once()

	ethClient := relayermocks.NewEthereumBridgeClient(t)
	ethClient.EXPECT().WatchDepositEvents(mock.Anything, uint64(20), mock.Anything).Return(nil).Once()

	engine := relayer.NewEngine(cfg, cantonClient, ethClient, store, zap.NewNop())
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	engine.Stop()
	if engine.IsReady() {
		t.Fatalf("engine should not be marked ready immediately")
	}
}
