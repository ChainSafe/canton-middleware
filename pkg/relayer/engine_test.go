package relayer

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/config"
)

func TestEngine_IsReady_InitiallyFalse(t *testing.T) {
	engine := NewEngine(&config.Config{}, nil, nil, nil, zap.NewNop())
	if engine.IsReady() {
		t.Error("engine should not be ready initially")
	}
}

func TestEngine_IsReady_BothSynced(t *testing.T) {
	engine := NewEngine(&config.Config{}, nil, nil, nil, zap.NewNop())
	engine.mu.Lock()
	engine.cantonSynced = true
	engine.ethereumSynced = true
	engine.mu.Unlock()

	if !engine.IsReady() {
		t.Error("engine should be ready when both chains are synced")
	}
}

func TestEngine_IsReady_OnlyEthereumSynced(t *testing.T) {
	engine := NewEngine(&config.Config{}, nil, nil, nil, zap.NewNop())
	engine.mu.Lock()
	engine.ethereumSynced = true
	engine.mu.Unlock()

	if engine.IsReady() {
		t.Error("engine should not be ready when only Ethereum is synced")
	}
}

func TestEngine_IsReady_OnlyCantonSynced(t *testing.T) {
	engine := NewEngine(&config.Config{}, nil, nil, nil, zap.NewNop())
	engine.mu.Lock()
	engine.cantonSynced = true
	engine.mu.Unlock()

	if engine.IsReady() {
		t.Error("engine should not be ready when only Canton is synced")
	}
}

func TestEngine_LoadOffsets_WithStoredState(t *testing.T) {
	cfg := &config.Config{}
	cfg.Canton.LookbackBlocks = 1000

	mockStore := &MockStore{
		GetChainStateFunc: func(_ context.Context, chainID string) (*ChainState, error) {
			switch chainID {
			case ChainCanton:
				return &ChainState{ChainID: ChainCanton, Offset: "5000"}, nil
			case ChainEthereum:
				return &ChainState{ChainID: ChainEthereum, LastBlock: 12345}, nil
			}
			return nil, nil
		},
	}

	mockCanton := &MockCantonClient{
		GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 7000, nil },
	}

	engine := NewEngine(cfg, mockCanton, &MockEthereumClient{}, mockStore, zap.NewNop())
	if err := engine.loadOffsets(context.Background()); err != nil {
		t.Fatalf("loadOffsets failed: %v", err)
	}
	if engine.cantonOffset != "5000" {
		t.Errorf("expected canton offset 5000, got %s", engine.cantonOffset)
	}
	if engine.ethLastBlock != 12345 {
		t.Errorf("expected eth block 12345, got %d", engine.ethLastBlock)
	}
}

func TestEngine_LoadOffsets_NoState_WithLookback(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ethereum.LookbackBlocks = 100
	cfg.Canton.LookbackBlocks = 200

	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 10000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 5000, nil },
		},
		&MockStore{GetChainStateFunc: func(_ context.Context, _ string) (*ChainState, error) { return nil, nil }},
		zap.NewNop(),
	)

	if err := engine.loadOffsets(context.Background()); err != nil {
		t.Fatalf("loadOffsets failed: %v", err)
	}
	if engine.cantonOffset != "9800" { // 10000 - 200
		t.Errorf("expected canton offset 9800, got %s", engine.cantonOffset)
	}
	if engine.ethLastBlock != 4900 { // 5000 - 100
		t.Errorf("expected eth block 4900, got %d", engine.ethLastBlock)
	}
}

func TestEngine_LoadOffsets_NoState_StartBlockOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ethereum.StartBlock = 1000
	cfg.Ethereum.LookbackBlocks = 100
	cfg.Canton.StartBlock = 2000
	cfg.Canton.LookbackBlocks = 200

	engine := NewEngine(cfg, &MockCantonClient{}, &MockEthereumClient{},
		&MockStore{GetChainStateFunc: func(_ context.Context, _ string) (*ChainState, error) { return nil, nil }},
		zap.NewNop(),
	)

	if err := engine.loadOffsets(context.Background()); err != nil {
		t.Fatalf("loadOffsets failed: %v", err)
	}
	if engine.cantonOffset != "2000" {
		t.Errorf("expected canton offset 2000 (from start_block), got %s", engine.cantonOffset)
	}
	if engine.ethLastBlock != 1000 {
		t.Errorf("expected eth block 1000 (from start_block), got %d", engine.ethLastBlock)
	}
}

func TestEngine_LoadOffsets_NoState_LookbackLargerThanChain(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ethereum.LookbackBlocks = 10000
	cfg.Canton.LookbackBlocks = 20000

	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 5000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 1000, nil },
		},
		&MockStore{GetChainStateFunc: func(_ context.Context, _ string) (*ChainState, error) { return nil, nil }},
		zap.NewNop(),
	)

	if err := engine.loadOffsets(context.Background()); err != nil {
		t.Fatalf("loadOffsets failed: %v", err)
	}
	if engine.cantonOffset != OffsetBegin {
		t.Errorf("expected canton offset BEGIN when lookback > ledger end, got %s", engine.cantonOffset)
	}
	if engine.ethLastBlock != 0 {
		t.Errorf("expected eth block 0 when lookback > chain height, got %d", engine.ethLastBlock)
	}
}

func TestEngine_LoadOffsets_NoState_LookbackDisabled(t *testing.T) {
	cfg := &config.Config{}

	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 10000, nil },
		},
		&MockEthereumClient{},
		&MockStore{GetChainStateFunc: func(_ context.Context, _ string) (*ChainState, error) { return nil, nil }},
		zap.NewNop(),
	)

	if err := engine.loadOffsets(context.Background()); err != nil {
		t.Fatalf("loadOffsets failed: %v", err)
	}
	if engine.cantonOffset != "10000" {
		t.Errorf("expected canton offset 10000 (ledger end, lookback=0), got %s", engine.cantonOffset)
	}
	if engine.ethLastBlock != 0 {
		t.Errorf("expected eth block 0 (genesis, lookback=0), got %d", engine.ethLastBlock)
	}
}

func TestEngine_CheckReadiness_EthereumCaughtUp(t *testing.T) {
	cfg := &config.Config{}
	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 1000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 100, nil },
		},
		nil, zap.NewNop(),
	)
	engine.ethLastBlock = 100

	engine.checkReadiness(context.Background())

	engine.mu.RLock()
	ethSynced := engine.ethereumSynced
	engine.mu.RUnlock()

	if !ethSynced {
		t.Error("ethereum should be marked as synced when at head")
	}
}

func TestEngine_CheckReadiness_EthereumBehind(t *testing.T) {
	cfg := &config.Config{}
	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 1000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 100, nil },
		},
		nil, zap.NewNop(),
	)
	engine.ethLastBlock = 50

	engine.checkReadiness(context.Background())

	engine.mu.RLock()
	ethSynced := engine.ethereumSynced
	engine.mu.RUnlock()

	if ethSynced {
		t.Error("ethereum should not be synced when 50 blocks behind")
	}
}

func TestEngine_CheckReadiness_CantonCaughtUp(t *testing.T) {
	cfg := &config.Config{}
	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 1000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 100, nil },
		},
		nil, zap.NewNop(),
	)
	engine.cantonOffset = "1000"

	engine.checkReadiness(context.Background())

	engine.mu.RLock()
	cantonSynced := engine.cantonSynced
	engine.mu.RUnlock()

	if !cantonSynced {
		t.Error("canton should be synced when at ledger end")
	}
}

func TestEngine_CheckReadiness_CantonBehind(t *testing.T) {
	cfg := &config.Config{}
	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 1000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 100, nil },
		},
		nil, zap.NewNop(),
	)
	engine.cantonOffset = "500"

	engine.checkReadiness(context.Background())

	engine.mu.RLock()
	cantonSynced := engine.cantonSynced
	engine.mu.RUnlock()

	if cantonSynced {
		t.Error("canton should not be synced when behind (within grace period)")
	}
}

func TestEngine_CheckReadiness_SyncedStaysTrue(t *testing.T) {
	cfg := &config.Config{}
	engine := NewEngine(cfg,
		&MockCantonClient{
			GetLatestLedgerOffsetFunc: func(_ context.Context) (int64, error) { return 2000, nil },
		},
		&MockEthereumClient{
			GetLatestBlockNumberFunc: func(_ context.Context) (uint64, error) { return 200, nil },
		},
		nil, zap.NewNop(),
	)

	engine.mu.Lock()
	engine.ethereumSynced = true
	engine.cantonSynced = true
	engine.mu.Unlock()

	engine.ethLastBlock = 100
	engine.cantonOffset = "1000"

	engine.checkReadiness(context.Background())

	engine.mu.RLock()
	ethSynced := engine.ethereumSynced
	cantonSynced := engine.cantonSynced
	engine.mu.RUnlock()

	if !ethSynced || !cantonSynced {
		t.Error("once synced, engine should remain synced (monotonic flag)")
	}
}
