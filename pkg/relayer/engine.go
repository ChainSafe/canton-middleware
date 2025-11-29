package relayer

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/internal/metrics"
	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/db"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

// CantonBridgeClient defines the interface for Canton interactions
type CantonBridgeClient interface {
	StreamBurnEvents(ctx context.Context, startOffset string) (<-chan *canton.BurnEvent, <-chan error)
	SubmitMintProposal(ctx context.Context, req *canton.MintProposalRequest) error
}

// EthereumBridgeClient defines the interface for Ethereum interactions
type EthereumBridgeClient interface {
	GetLatestBlockNumber(ctx context.Context) (uint64, error)
	WithdrawFromCanton(ctx context.Context, token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (common.Hash, error)
}

// BridgeStore defines the interface for database operations
type BridgeStore interface {
	GetTransfer(id string) (*db.Transfer, error)
	CreateTransfer(transfer *db.Transfer) error
	UpdateTransferStatus(id string, status db.TransferStatus, destTxHash *string) error
	GetChainState(chainID string) (*db.ChainState, error)
	SetChainState(chainID string, blockNumber int64, blockHash string) error
	GetPendingTransfers(direction db.TransferDirection) ([]*db.Transfer, error)
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

	// Initialize processors
	cantonSource := NewCantonSource(e.cantonClient)
	ethDest := NewEthereumDestination(e.ethClient)
	cantonProcessor := NewProcessor(cantonSource, ethDest, e.store, e.logger, "canton_processor")

	ethSource := NewEthereumSource(e.ethClient, &e.config.Ethereum)
	cantonDest := NewCantonDestination(e.cantonClient, &e.config.Ethereum, e.config.Canton.RelayerParty)
	ethProcessor := NewProcessor(ethSource, cantonDest, e.store, e.logger, "ethereum_processor")

	// Start Canton processor
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		if err := cantonProcessor.Start(ctx, e.cantonOffset); err != nil {
			e.logger.Error("Canton processor failed", zap.Error(err))
		}
	}()

	// Start Ethereum processor
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		// Convert uint64 block to string offset
		ethOffset := fmt.Sprintf("%d", e.ethLastBlock)
		if err := ethProcessor.Start(ctx, ethOffset); err != nil {
			e.logger.Error("Ethereum processor failed", zap.Error(err))
		}
	}()

	// Start periodic reconciliation
	e.wg.Add(1)
	go e.reconcile(ctx)

	e.logger.Info("Relayer engine started")
	return nil
}

// Stop stops the relayer engine
func (e *Engine) Stop() {
	e.logger.Info("Stopping relayer engine")
	close(e.stopCh)
	e.wg.Wait()
	e.logger.Info("Relayer engine stopped")
}

// loadOffsets loads last processed offsets from database
func (e *Engine) loadOffsets(_ context.Context) error {
	// Load Canton offset
	cantonState, err := e.store.GetChainState("canton")
	if err != nil {
		return fmt.Errorf("failed to get Canton state: %w", err)
	}
	if cantonState != nil {
		e.cantonOffset = cantonState.LastBlockHash
		e.logger.Info("Loaded Canton offset", zap.String("offset", e.cantonOffset))
	} else {
		e.cantonOffset = "BEGIN"
		e.logger.Info("Starting Canton stream from beginning")
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
		e.ethLastBlock = uint64(e.config.Ethereum.StartBlock)
		e.logger.Info("Starting Ethereum from configured block", zap.Uint64("block", e.ethLastBlock))
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

// runReconciliation checks for stuck transfers and retries
func (e *Engine) runReconciliation(ctx context.Context) error {
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
