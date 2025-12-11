package relayer

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/db"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

// CantonBridgeClient defines the interface for Canton interactions
type CantonBridgeClient interface {
	// Legacy methods (deprecated but kept for backwards compatibility)
	StreamBurnEvents(ctx context.Context, startOffset string) (<-chan *canton.BurnEvent, <-chan error)
	SubmitMintProposal(ctx context.Context, req *canton.MintProposalRequest) error

	// Issuer-centric model methods (new)
	StreamWithdrawalEvents(ctx context.Context, offset string) (<-chan *canton.WithdrawalEvent, <-chan error)
	RegisterUser(ctx context.Context, req *canton.RegisterUserRequest) (string, error)
	GetFingerprintMapping(ctx context.Context, fingerprint string) (*canton.FingerprintMapping, error)
	CreatePendingDeposit(ctx context.Context, req *canton.CreatePendingDepositRequest) (string, error)
	ProcessDeposit(ctx context.Context, req *canton.ProcessDepositRequest) (string, error)
	InitiateWithdrawal(ctx context.Context, req *canton.InitiateWithdrawalRequest) (string, error)
	CompleteWithdrawal(ctx context.Context, req *canton.CompleteWithdrawalRequest) error

	// Ledger state
	GetLedgerEnd(ctx context.Context) (string, error)
}

// EthereumBridgeClient defines the interface for Ethereum interactions
type EthereumBridgeClient interface {
	GetLatestBlockNumber(ctx context.Context) (uint64, error)
	WithdrawFromCanton(ctx context.Context, token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (common.Hash, error)
	WatchDepositEvents(ctx context.Context, fromBlock uint64, handler func(*ethereum.DepositEvent) error) error
	IsWithdrawalProcessed(ctx context.Context, cantonTxHash [32]byte) (bool, error)
}

// BridgeStore defines the interface for database operations
type BridgeStore interface {
	GetTransfer(id string) (*db.Transfer, error)
	CreateTransfer(transfer *db.Transfer) error
	UpdateTransferStatus(id string, status db.TransferStatus, destTxHash *string) error
	GetChainState(chainID string) (*db.ChainState, error)
	SetChainState(chainID string, blockNumber int64, blockHash string) error
	GetPendingTransfers(direction db.TransferDirection) ([]*db.Transfer, error)
	ListTransfers(limit int) ([]*db.Transfer, error)
}

// Engine orchestrates the bridge relayer operations
type Engine struct {
	config       *config.Config
	cantonClient CantonBridgeClient
	ethClient    EthereumBridgeClient
	store        BridgeStore
	logger       *zap.Logger

	cantonOffset string
	ethLastBlock uint64

	stopCh chan struct{}
	wg     sync.WaitGroup

	// Readiness tracking - protected by mu
	mu             sync.RWMutex
	cantonSynced   bool
	ethereumSynced bool
}

// NewEngine creates a new relayer engine
func NewEngine(
	cfg *config.Config,
	cantonClient CantonBridgeClient,
	ethClient EthereumBridgeClient,
	store BridgeStore,
	logger *zap.Logger,
) *Engine {
	return &Engine{
		config:       cfg,
		cantonClient: cantonClient,
		ethClient:    ethClient,
		store:        store,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

// Start starts the relayer engine
func (e *Engine) Start(ctx context.Context) error {
	e.logger.Info("Starting relayer engine")

	// Initialize offsets from database
	if err := e.loadOffsets(ctx); err != nil {
		return fmt.Errorf("failed to load offsets: %w", err)
	}

	// Initialize processors with offset persistence callbacks
	cantonSource := NewCantonSource(e.cantonClient, e.config.Ethereum.TokenContract)
	ethDest := NewEthereumDestination(e.ethClient, e.cantonClient)
	cantonProcessor := NewProcessor(cantonSource, ethDest, e.store, e.logger, "canton_processor").
		WithOffsetUpdate(e.saveChainOffset)

	ethSource := NewEthereumSource(e.ethClient, &e.config.Ethereum)
	cantonDest := NewCantonDestination(e.cantonClient, &e.config.Ethereum, e.config.Canton.RelayerParty)
	ethProcessor := NewProcessor(ethSource, cantonDest, e.store, e.logger, "ethereum_processor").
		WithOffsetUpdate(e.saveChainOffset)

	// Start Canton processor
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		if err := cantonProcessor.Start(ctx, e.cantonOffset); err != nil {
			// Canton streaming may fail due to protobuf version mismatch with Canton 3.4.8
			// The update_format field is required but not in our generated protos
			// This is a known issue - regenerate protos from Canton 3.4.8 to fix
			e.logger.Warn("Canton processor stopped (protobuf update needed for Canton 3.4.8)",
				zap.Error(err),
				zap.String("hint", "Regenerate protos from Canton 3.4.8 to enable withdrawal streaming"))
		}
	}()

	// Start Ethereum processor
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		// Convert uint64 block to string offset
		ethOffset := fmt.Sprintf("%d", e.ethLastBlock)
		if err := ethProcessor.Start(ctx, ethOffset); err != nil {
			e.logger.Warn("Ethereum processor stopped", zap.Error(err))
		}
	}()

	// Start periodic reconciliation
	e.wg.Add(1)
	go e.reconcile(ctx)

	// Start readiness checker
	e.wg.Add(1)
	go e.readinessLoop(ctx)

	e.logger.Info("Relayer engine started")
	return nil
}

// IsReady returns true once both Canton and Ethereum processors have
// caught up to head at least once.
func (e *Engine) IsReady() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cantonSynced && e.ethereumSynced
}

// Stop stops the relayer engine
func (e *Engine) Stop() {
	e.logger.Info("Stopping relayer engine")
	close(e.stopCh)
	e.wg.Wait()
	e.logger.Info("Relayer engine stopped")
}

// loadOffsets loads last processed offsets from database
func (e *Engine) loadOffsets(ctx context.Context) error {
	// Load Canton offset
	cantonState, err := e.store.GetChainState("canton")
	if err != nil {
		return fmt.Errorf("failed to get Canton state: %w", err)
	}
	if cantonState != nil {
		e.cantonOffset = cantonState.LastBlockHash
		e.logger.Info("Loaded Canton offset", zap.String("offset", e.cantonOffset))
	} else {
		// No stored state for Canton
		// 1) Backwards-compatible override: explicit start_block wins
		if e.config.Canton.StartBlock > 0 {
			e.cantonOffset = strconv.FormatInt(e.config.Canton.StartBlock, 10)
			e.logger.Info("Starting Canton stream from configured start_block",
				zap.String("offset", e.cantonOffset))
		} else {
			// 2) Use ledger end minus lookback_blocks
			ledgerEnd, err := e.cantonClient.GetLedgerEnd(ctx)
			if err != nil {
				e.logger.Warn("Failed to get Canton ledger end, falling back to BEGIN",
					zap.Error(err))
				e.cantonOffset = "BEGIN"
			} else {
				lookback := e.config.Canton.LookbackBlocks
				if lookback <= 0 {
					// lookback <= 0: preserve old behavior (start at tip, no replay)
					e.cantonOffset = ledgerEnd
					e.logger.Info("Starting Canton stream from current ledger end (lookback disabled)",
						zap.String("ledger_end", ledgerEnd))
				} else {
					endOffset, err := strconv.ParseInt(ledgerEnd, 10, 64)
					if err != nil {
						e.logger.Warn("Invalid Canton ledger end value, falling back to BEGIN",
							zap.String("ledger_end", ledgerEnd),
							zap.Error(err))
						e.cantonOffset = "BEGIN"
					} else {
						if endOffset <= lookback {
							// Not enough history to look back fully; start from BEGIN
							e.cantonOffset = "BEGIN"
						} else {
							e.cantonOffset = strconv.FormatInt(endOffset-lookback, 10)
						}
						e.logger.Info("Starting Canton stream from ledger end with lookback",
							zap.String("ledger_end", ledgerEnd),
							zap.Int64("lookback_blocks", lookback),
							zap.String("start_offset", e.cantonOffset))
					}
				}
			}
		}
	}

	// Load Ethereum last block
	ethState, err := e.store.GetChainState("ethereum")
	if err != nil {
		return fmt.Errorf("failed to get Ethereum state: %w", err)
	}
	if ethState != nil {
		e.ethLastBlock = uint64(ethState.LastBlock)
		e.logger.Info("Loaded Ethereum last block", zap.Uint64("block", e.ethLastBlock))
	} else {
		// No stored state for Ethereum
		// 1) Backwards-compatible override: explicit start_block wins
		if e.config.Ethereum.StartBlock > 0 {
			e.ethLastBlock = uint64(e.config.Ethereum.StartBlock)
			e.logger.Info("Starting Ethereum from configured start_block",
				zap.Uint64("block", e.ethLastBlock))
		} else {
			lookback := e.config.Ethereum.LookbackBlocks
			if lookback <= 0 {
				// lookback <= 0: preserve old behavior (start from genesis)
				e.ethLastBlock = 0
				e.logger.Info("Starting Ethereum from genesis (lookback disabled)",
					zap.Uint64("block", e.ethLastBlock))
			} else {
				headBlock, err := e.ethClient.GetLatestBlockNumber(ctx)
				if err != nil {
					// Fallback to previous behavior if we can't query head
					e.ethLastBlock = uint64(e.config.Ethereum.StartBlock)
					e.logger.Warn("Failed to get Ethereum latest block, falling back to configured start_block",
						zap.Error(err),
						zap.Uint64("start_block", e.ethLastBlock))
				} else {
					if uint64(lookback) >= headBlock {
						// Lookback larger than chain height; start from genesis
						e.ethLastBlock = 0
					} else {
						e.ethLastBlock = headBlock - uint64(lookback)
					}
					e.logger.Info("Starting Ethereum from latest block with lookback",
						zap.Uint64("head_block", headBlock),
						zap.Int64("lookback_blocks", lookback),
						zap.Uint64("start_block", e.ethLastBlock))
				}
			}
		}
	}

	return nil
}

// reconcile periodically reconciles bridge state
func (e *Engine) reconcile(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			if err := e.runReconciliation(ctx); err != nil {
				e.logger.Error("Reconciliation failed", zap.Error(err))
			}
		}
	}
}

// saveChainOffset persists the last processed offset for a chain
func (e *Engine) saveChainOffset(chainID string, offset string) error {
	// For Canton, we store offset in LastBlockHash (it's a string offset like "12345")
	// For Ethereum, we parse the block number
	var blockNumber int64
	if chainID == "ethereum" {
		if n, err := fmt.Sscanf(offset, "%d", &blockNumber); err != nil || n != 1 {
			return fmt.Errorf("invalid ethereum offset: %s", offset)
		}
	}
	
	if err := e.store.SetChainState(chainID, blockNumber, offset); err != nil {
		return fmt.Errorf("failed to save chain state for %s: %w", chainID, err)
	}
	
	e.logger.Debug("Saved chain offset",
		zap.String("chain", chainID),
		zap.String("offset", offset))
	
	return nil
}

// runReconciliation checks for stuck transfers and retries
func (e *Engine) runReconciliation(_ context.Context) error {
	e.logger.Info("Running reconciliation")

	// Get pending Canton->Ethereum transfers
	cantonPending, err := e.store.GetPendingTransfers(db.DirectionCantonToEthereum)
	if err != nil {
		return fmt.Errorf("failed to get pending Canton transfers: %w", err)
	}

	// Get pending Ethereum->Canton transfers
	ethPending, err := e.store.GetPendingTransfers(db.DirectionEthereumToCanton)
	if err != nil {
		return fmt.Errorf("failed to get pending Ethereum transfers: %w", err)
	}

	e.logger.Info("Reconciliation summary",
		zap.Int("canton_pending", len(cantonPending)),
		zap.Int("ethereum_pending", len(ethPending)))

	// TODO: Retry failed transfers
	// TODO: Alert on stuck transfers

	metrics.PendingTransfers.WithLabelValues("canton_to_ethereum").Set(float64(len(cantonPending)))
	metrics.PendingTransfers.WithLabelValues("ethereum_to_canton").Set(float64(len(ethPending)))

	return nil
}

// readinessLoop periodically checks if the engine has caught up to head
func (e *Engine) readinessLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.checkReadiness(ctx)
			// Stop checking once fully ready
			if e.IsReady() {
				e.logger.Info("Relayer is fully synced and ready")
				return
			}
		}
	}
}

// checkReadiness checks if both chains have caught up to their respective heads
func (e *Engine) checkReadiness(ctx context.Context) {
	// Check Ethereum readiness
	headBlock, err := e.ethClient.GetLatestBlockNumber(ctx)
	if err != nil {
		e.logger.Warn("Failed to get Ethereum latest block for readiness check", zap.Error(err))
	} else {
		e.mu.Lock()
		if !e.ethereumSynced {
			// Allow 1 block lag tolerance
			if e.ethLastBlock+1 >= headBlock {
				e.ethereumSynced = true
				e.logger.Info("Ethereum processor initial sync complete",
					zap.Uint64("last_block", e.ethLastBlock),
					zap.Uint64("head_block", headBlock))
			} else {
				e.logger.Debug("Ethereum processor catching up",
					zap.Uint64("last_block", e.ethLastBlock),
					zap.Uint64("head_block", headBlock),
					zap.Uint64("blocks_behind", headBlock-e.ethLastBlock))
			}
		}
		e.mu.Unlock()
	}

	// Check Canton readiness
	ledgerEnd, err := e.cantonClient.GetLedgerEnd(ctx)
	if err != nil {
		e.logger.Warn("Failed to get Canton ledger end for readiness check", zap.Error(err))
	} else {
		e.mu.Lock()
		if !e.cantonSynced {
			// If we started from ledger end or have no offset, consider synced
			if e.cantonOffset == "" || e.cantonOffset == ledgerEnd {
				e.cantonSynced = true
				e.logger.Info("Canton processor initial sync complete (at ledger end)",
					zap.String("offset", e.cantonOffset))
			} else if e.cantonOffset != "BEGIN" {
				// Try numeric comparison for Canton offsets
				currentOffset, err1 := strconv.ParseInt(e.cantonOffset, 10, 64)
				endOffset, err2 := strconv.ParseInt(ledgerEnd, 10, 64)
				if err1 == nil && err2 == nil {
					if currentOffset >= endOffset {
						e.cantonSynced = true
						e.logger.Info("Canton processor initial sync complete",
							zap.String("offset", e.cantonOffset),
							zap.String("ledger_end", ledgerEnd))
					} else {
						e.logger.Debug("Canton processor catching up",
							zap.String("offset", e.cantonOffset),
							zap.String("ledger_end", ledgerEnd),
							zap.Int64("behind", endOffset-currentOffset))
					}
				}
			}
		}
		e.mu.Unlock()
	}
}
