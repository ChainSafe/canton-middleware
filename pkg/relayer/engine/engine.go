package engine

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ethereum/go-ethereum/common"

	"github.com/chainsafe/canton-middleware/internal/metrics"
	canton "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// stuckTransferThreshold is the age beyond which a pending transfer is considered stuck
// and eligible for retry during reconciliation.
const stuckTransferThreshold = 15 * time.Minute

const (
	cantonProcessorRestartInitialBackoff = 1 * time.Second
	cantonProcessorRestartMaxBackoff     = 30 * time.Second
)

// EthereumBridgeClient defines the interface for Ethereum interactions.
//
//go:generate mockery --name EthereumBridgeClient --output mocks --outpkg mocks --filename mock_ethereum_bridge_client.go --with-expecter
type EthereumBridgeClient interface {
	GetLatestBlockNumber(ctx context.Context) (uint64, error)
	WithdrawFromCanton(
		ctx context.Context,
		token common.Address,
		recipient common.Address,
		amount *big.Int,
		nonce *big.Int,
		cantonTxHash [32]byte,
	) (common.Hash, error)
	WatchDepositEvents(ctx context.Context, fromBlock uint64, handler func(*ethereum.DepositEvent) error) error
	IsWithdrawalProcessed(ctx context.Context, cantonTxHash [32]byte) (bool, error)
	GetLastScannedBlock() uint64
}

// CantonBridgeClient defines the Canton bridge interactions consumed by Engine.
//
//go:generate mockery --name CantonBridgeClient --output mocks --outpkg mocks --filename mock_canton_bridge.go --structname CantonBridge --with-expecter
type CantonBridgeClient interface {
	canton.Bridge
}

// BridgeStore defines the narrow data-access interface consumed by the relayer.
//
//go:generate mockery --name BridgeStore --output mocks --outpkg mocks --filename mock_bridge_store.go --with-expecter
type BridgeStore interface {
	// CreateTransfer inserts a new transfer record. Returns true if the record was newly
	// inserted, false if it already existed (idempotent via ON CONFLICT DO NOTHING).
	CreateTransfer(ctx context.Context, transfer *relayer.Transfer) (bool, error)
	GetTransfer(ctx context.Context, id string) (*relayer.Transfer, error)
	// UpdateTransferStatus updates status, destination tx hash, and optionally error message.
	UpdateTransferStatus(ctx context.Context, id string, status relayer.TransferStatus, destTxHash *string, errMsg *string) error
	// IncrementRetryCount atomically increments the retry_count for a transfer.
	IncrementRetryCount(ctx context.Context, id string) error
	GetChainState(ctx context.Context, chainID string) (*relayer.ChainState, error)
	SetChainState(ctx context.Context, chainID string, blockNumber int64, offset string) error
	GetPendingTransfers(ctx context.Context, direction relayer.TransferDirection) ([]*relayer.Transfer, error)
	ListTransfers(ctx context.Context, limit int) ([]*relayer.Transfer, error)
}

// Engine orchestrates the bridge relayer operations.
type Engine struct {
	config       *config.Config
	cantonClient CantonBridgeClient
	ethClient    EthereumBridgeClient
	store        BridgeStore
	logger       *zap.Logger

	// Cached chain keys (computed once from config).
	cantonKey   string
	ethereumKey string

	cantonOffset string
	ethLastBlock uint64

	// Destinations held for reconciliation retries.
	ethDest    Destination
	cantonDest Destination

	// cancel stops all internal goroutines; set by Start.
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Readiness tracking — protected by mu.
	mu                  sync.RWMutex
	cantonSynced        bool
	ethereumSynced      bool
	cantonStreamStarted time.Time
}

// NewEngine creates a new relayer engine.
func NewEngine(
	cfg *config.Config,
	cantonClient CantonBridgeClient,
	ethClient EthereumBridgeClient,
	store BridgeStore,
	logger *zap.Logger,
) *Engine {
	cantonKey := relayer.ChainCanton
	if cfg.Canton.ChainID != "" {
		cantonKey = cfg.Canton.ChainID
	}

	ethereumKey := relayer.ChainEthereum
	if cfg.Ethereum.ChainID != 0 {
		ethereumKey = strconv.FormatInt(cfg.Ethereum.ChainID, 10)
	}

	return &Engine{
		config:       cfg,
		cantonClient: cantonClient,
		ethClient:    ethClient,
		store:        store,
		logger:       logger,
		cantonKey:    cantonKey,
		ethereumKey:  ethereumKey,
	}
}

// Start starts the relayer engine. It wraps ctx so that Stop() can cancel all goroutines.
func (e *Engine) Start(ctx context.Context) error {
	e.logger.Info("Starting relayer engine")
	ctx, e.cancel = context.WithCancel(ctx)

	if err := e.loadOffsets(ctx); err != nil {
		return fmt.Errorf("failed to load offsets: %w", err)
	}

	cantonSrc := NewCantonSource(e.cantonClient, e.config.Ethereum.TokenContract, e.cantonKey)
	e.ethDest = NewEthereumDestination(e.ethClient, e.ethereumKey, e.logger)

	ethSource := NewEthereumSource(e.ethClient, &e.config.Ethereum, e.ethereumKey)
	e.cantonDest = NewCantonDestination(e.cantonClient, e.cantonKey, e.logger)

	// After each Canton→Ethereum withdrawal is submitted on EVM, mark it complete on Canton.
	completeWithdrawal := func(ctx context.Context, event *relayer.Event, destTxHash string) error {
		if event.SourceContractID == "" || e.cantonClient == nil {
			return nil
		}
		if err := e.cantonClient.CompleteWithdrawal(ctx, canton.CompleteWithdrawalRequest{
			WithdrawalEventCID: event.SourceContractID,
			EvmTxHash:          destTxHash,
		}); err != nil {
			e.logger.Warn("Failed to complete withdrawal on Canton (EVM tx succeeded, reconcile later)",
				zap.String("contract_id", event.SourceContractID), zap.Error(err))
		}
		return nil // best-effort: do not fail the transfer
	}

	cantonProcessor := NewProcessor(cantonSrc, e.ethDest, e.store, e.logger, "canton_processor", relayer.DirectionCantonToEthereum).
		WithOffsetUpdate(e.saveChainOffset).
		WithPostSubmit(completeWithdrawal)
	ethProcessor := NewProcessor(ethSource, e.cantonDest, e.store, e.logger, "ethereum_processor", relayer.DirectionEthereumToCanton).
		WithOffsetUpdate(e.saveChainOffset)

	e.wg.Add(1)
	go e.runCantonProcessorLoop(ctx, cantonProcessor)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ethOffset := strconv.FormatUint(e.ethLastBlock, 10)
		if err := ethProcessor.Start(ctx, ethOffset); err != nil {
			e.logger.Warn("Ethereum processor stopped", zap.Error(err))
		}
	}()

	e.wg.Add(1)
	go e.reconcileLoop(ctx)

	e.wg.Add(1)
	go e.readinessLoop(ctx)

	e.logger.Info("Relayer engine started")
	return nil
}

func (e *Engine) runCantonProcessorLoop(ctx context.Context, cantonProcessor *Processor) {
	defer e.wg.Done()

	backoff := cantonProcessorRestartInitialBackoff

	for {
		// Record when the Canton stream actually starts (not when Start() was called).
		e.mu.Lock()
		e.cantonStreamStarted = time.Now()
		startOffset := e.cantonOffset
		e.mu.Unlock()

		err := cantonProcessor.Start(ctx, startOffset)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}

		if err != nil {
			e.logger.Warn("Canton processor stopped; restarting with backoff",
				zap.Error(err),
				zap.String("offset", startOffset),
				zap.Duration("restart_in", backoff),
				zap.String("hint", "Regenerate protos from Canton 3.4.8 to enable withdrawal streaming"))
		} else {
			e.logger.Warn("Canton processor exited unexpectedly; restarting with backoff",
				zap.String("offset", startOffset),
				zap.Duration("restart_in", backoff))
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}

		backoff = nextCantonProcessorBackoff(backoff)
	}
}

func nextCantonProcessorBackoff(current time.Duration) time.Duration {
	if current <= 0 {
		return cantonProcessorRestartInitialBackoff
	}

	next := current * 2
	if next > cantonProcessorRestartMaxBackoff {
		return cantonProcessorRestartMaxBackoff
	}
	return next
}

// IsReady returns true once both Canton and Ethereum processors have caught up to head.
func (e *Engine) IsReady() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cantonSynced && e.ethereumSynced
}

// Stop cancels all goroutines started by Start and waits for them to finish.
func (e *Engine) Stop() {
	e.logger.Info("Stopping relayer engine")
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	e.logger.Info("Relayer engine stopped")
}

// loadOffsets loads last processed offsets from the database for both chains.
func (e *Engine) loadOffsets(ctx context.Context) error {
	if err := e.loadCantonOffset(ctx); err != nil {
		return err
	}
	return e.loadEthereumOffset(ctx)
}

func (e *Engine) loadCantonOffset(ctx context.Context) error {
	state, err := e.store.GetChainState(ctx, e.cantonKey)
	if err != nil {
		return fmt.Errorf("get canton chain state: %w", err)
	}

	if state != nil {
		return e.validateStoredCantonOffset(ctx, state.Offset)
	}

	// No stored state — determine start offset from config.
	if e.config.Canton.StartBlock > 0 {
		e.cantonOffset = strconv.FormatInt(e.config.Canton.StartBlock, 10)
		e.logger.Info("Starting Canton stream from configured start_block", zap.String("offset", e.cantonOffset))
		return nil
	}

	return e.cantonOffsetFromLedgerEnd(ctx)
}

func (e *Engine) validateStoredCantonOffset(ctx context.Context, storedOffset string) error {
	ledgerEnd, err := e.cantonClient.GetLatestLedgerOffset(ctx)
	if err != nil {
		e.logger.Warn("Failed to validate Canton offset against ledger end, using stored offset",
			zap.String("stored_offset", storedOffset), zap.Error(err))
		e.cantonOffset = storedOffset
		return nil
	}

	storedNum, parseErr := strconv.ParseInt(storedOffset, 10, 64)
	if parseErr != nil || storedNum <= ledgerEnd {
		e.cantonOffset = storedOffset
		e.logger.Info("Loaded Canton offset", zap.String("offset", e.cantonOffset))
		return nil
	}

	// Stored offset is ahead of ledger end — reset to a safe position.
	safeOffset := e.safeCantonOffset(ledgerEnd)
	e.logger.Warn("Stored Canton offset is ahead of ledger end, resetting",
		zap.String("stored_offset", storedOffset),
		zap.Int64("ledger_end", ledgerEnd),
		zap.String("safe_offset", safeOffset))
	e.cantonOffset = safeOffset
	if err = e.store.SetChainState(ctx, e.cantonKey, 0, safeOffset); err != nil {
		return fmt.Errorf("persist corrected Canton offset: %w", err)
	}
	return nil
}

func (e *Engine) cantonOffsetFromLedgerEnd(ctx context.Context) error {
	ledgerEnd, err := e.cantonClient.GetLatestLedgerOffset(ctx)
	if err != nil || ledgerEnd == 0 {
		e.logger.Warn("Failed to get Canton ledger end, falling back to BEGIN", zap.Error(err))
		e.cantonOffset = relayer.OffsetBegin
		return nil
	}

	lookback := e.config.Canton.LookbackBlocks
	if lookback <= 0 {
		e.cantonOffset = strconv.FormatInt(ledgerEnd, 10)
		e.logger.Info("Starting Canton stream from current ledger end (lookback disabled)",
			zap.String("offset", e.cantonOffset))
		return nil
	}

	e.cantonOffset = e.safeCantonOffset(ledgerEnd)
	e.logger.Info("Starting Canton stream from ledger end with lookback",
		zap.Int64("ledger_end", ledgerEnd),
		zap.Int64("lookback_blocks", lookback),
		zap.String("start_offset", e.cantonOffset))
	return nil
}

func (e *Engine) safeCantonOffset(ledgerEnd int64) string {
	lookback := e.config.Canton.LookbackBlocks
	if lookback <= 0 {
		lookback = 1
	}
	if ledgerEnd <= lookback {
		return relayer.OffsetBegin
	}
	return strconv.FormatInt(ledgerEnd-lookback, 10)
}

func (e *Engine) loadEthereumOffset(ctx context.Context) error {
	state, err := e.store.GetChainState(ctx, e.ethereumKey)
	if err != nil {
		return fmt.Errorf("get ethereum chain state: %w", err)
	}

	if state != nil {
		e.ethLastBlock = uint64(state.LastBlock) //nolint:gosec // LastBlock is always non-negative
		e.logger.Info("Loaded Ethereum last block", zap.Uint64("block", e.ethLastBlock))
		return nil
	}

	// No stored state — determine start block from config.
	if e.config.Ethereum.StartBlock > 0 {
		e.ethLastBlock = uint64(e.config.Ethereum.StartBlock) //nolint:gosec // StartBlock is always non-negative
		e.logger.Info("Starting Ethereum from configured start_block", zap.Uint64("block", e.ethLastBlock))
		return nil
	}

	return e.ethBlockFromChainHead(ctx)
}

func (e *Engine) ethBlockFromChainHead(ctx context.Context) error {
	lookback := e.config.Ethereum.LookbackBlocks
	if lookback <= 0 {
		e.ethLastBlock = 0
		e.logger.Info("Starting Ethereum from genesis (lookback disabled)")
		return nil
	}

	headBlock, err := e.ethClient.GetLatestBlockNumber(ctx)
	if err != nil {
		e.ethLastBlock = uint64(e.config.Ethereum.StartBlock) //nolint:gosec // StartBlock is always non-negative
		e.logger.Warn("Failed to get Ethereum latest block, falling back to configured start_block",
			zap.Error(err), zap.Uint64("start_block", e.ethLastBlock))
		return nil
	}

	if uint64(lookback) >= headBlock {
		e.ethLastBlock = 0
	} else {
		e.ethLastBlock = headBlock - uint64(lookback)
	}
	e.logger.Info("Starting Ethereum from latest block with lookback",
		zap.Uint64("head_block", headBlock),
		zap.Int64("lookback_blocks", lookback),
		zap.Uint64("start_block", e.ethLastBlock))
	return nil
}

// saveChainOffset persists the last processed offset for a chain.
// The DB write happens first; in-memory state is only updated on success.
func (e *Engine) saveChainOffset(ctx context.Context, chainID string, offset string) error {
	var blockNumber int64

	if chainID == e.ethereumKey {
		n, err := strconv.ParseInt(offset, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ethereum offset %q: %w", offset, err)
		}
		blockNumber = n
	}

	if err := e.store.SetChainState(ctx, chainID, blockNumber, offset); err != nil {
		return fmt.Errorf("save chain state for %s: %w", chainID, err)
	}

	// Update in-memory state only after the DB write succeeds.
	e.mu.Lock()
	if chainID == e.ethereumKey {
		e.ethLastBlock = uint64(blockNumber) //nolint:gosec // block number parsed from our own offset string, always non-negative
	} else {
		e.cantonOffset = offset
	}
	e.mu.Unlock()

	e.logger.Debug("Saved chain offset", zap.String("chain", chainID), zap.String("offset", offset))
	return nil
}

// reconcileLoop periodically checks for stuck transfers and retries them.
func (e *Engine) reconcileLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.runReconciliation(ctx); err != nil {
				e.logger.Error("Reconciliation failed", zap.Error(err))
			}
		}
	}
}

// runReconciliation checks pending transfers, emits metrics, and retries stuck ones.
func (e *Engine) runReconciliation(ctx context.Context) error {
	e.logger.Info("Running reconciliation")

	cantonPending, err := e.store.GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum)
	if err != nil {
		return fmt.Errorf("get pending canton transfers: %w", err)
	}

	ethPending, err := e.store.GetPendingTransfers(ctx, relayer.DirectionEthereumToCanton)
	if err != nil {
		return fmt.Errorf("get pending ethereum transfers: %w", err)
	}

	e.logger.Info("Reconciliation summary",
		zap.Int("canton_pending", len(cantonPending)),
		zap.Int("ethereum_pending", len(ethPending)))

	metrics.PendingTransfers.WithLabelValues("canton_to_ethereum").Set(float64(len(cantonPending)))
	metrics.PendingTransfers.WithLabelValues("ethereum_to_canton").Set(float64(len(ethPending)))

	// Retry transfers stuck longer than RetryDelay, up to MaxRetries attempts.
	maxRetries := e.config.Bridge.MaxRetries
	retryDelay := e.config.Bridge.RetryDelay
	if retryDelay <= 0 {
		retryDelay = stuckTransferThreshold
	}

	for _, t := range cantonPending {
		e.maybeRetryTransfer(ctx, t, e.ethDest, maxRetries, retryDelay)
	}
	for _, t := range ethPending {
		e.maybeRetryTransfer(ctx, t, e.cantonDest, maxRetries, retryDelay)
	}

	return nil
}

// maybeRetryTransfer checks whether a pending transfer should be retried or given up on.
func (e *Engine) maybeRetryTransfer(ctx context.Context, t *relayer.Transfer, dest Destination, maxRetries int, retryDelay time.Duration) {
	if maxRetries > 0 && t.RetryCount >= maxRetries {
		e.logger.Warn("Transfer exceeded max retries, marking as failed",
			zap.String("id", t.ID),
			zap.Int("retry_count", t.RetryCount),
			zap.Int("max_retries", maxRetries))
		errMsg := fmt.Sprintf("max retries (%d) exceeded", maxRetries)
		if updateErr := e.store.UpdateTransferStatus(ctx, t.ID, relayer.TransferStatusFailed, nil, &errMsg); updateErr != nil {
			e.logger.Warn("Failed to mark transfer as failed after max retries", zap.String("id", t.ID), zap.Error(updateErr))
		}
		return
	}

	// Retry only after enough time has elapsed since last attempt.
	nextRetryAt := t.UpdatedAt.Add(retryDelay)
	if time.Now().Before(nextRetryAt) {
		return
	}

	e.retryStuckTransfer(ctx, t, dest)
}

// retryStuckTransfer attempts to resubmit a pending transfer that has been stuck.
// The Raw field is not available for reconstructed events; best-effort side effects
// (CompleteWithdrawal) are skipped on this path.
func (e *Engine) retryStuckTransfer(ctx context.Context, t *relayer.Transfer, dest Destination) {
	if dest == nil {
		e.logger.Error("No destination available for stuck transfer retry", zap.String("id", t.ID))
		return
	}

	e.logger.Warn("Retrying stuck transfer",
		zap.String("id", t.ID),
		zap.String("direction", string(t.Direction)),
		zap.Int("retry_count", t.RetryCount),
		zap.Duration("age", time.Since(t.CreatedAt)))

	event := &relayer.Event{
		ID:                t.ID,
		SourceChain:       t.SourceChain,
		DestinationChain:  t.DestinationChain,
		SourceTxHash:      t.SourceTxHash,
		TokenAddress:      t.TokenAddress,
		Amount:            t.Amount,
		Sender:            t.Sender,
		Recipient:         t.Recipient,
		Nonce:             t.Nonce,
		SourceBlockNumber: t.SourceBlockNumber,
	}

	destTxHash, skipped, err := dest.SubmitTransfer(ctx, event)
	if err != nil {
		e.logger.Error("Stuck transfer retry failed", zap.String("id", t.ID), zap.Error(err))
		if incrErr := e.store.IncrementRetryCount(ctx, t.ID); incrErr != nil {
			e.logger.Warn("Failed to increment retry count", zap.String("id", t.ID), zap.Error(incrErr))
		}
		return
	}

	var txHashPtr *string
	if !skipped {
		txHashPtr = &destTxHash
	}
	if updateErr := e.store.UpdateTransferStatus(ctx, t.ID, relayer.TransferStatusCompleted, txHashPtr, nil); updateErr != nil {
		e.logger.Warn("Failed to mark retried transfer as completed", zap.String("id", t.ID), zap.Error(updateErr))
	} else {
		e.logger.Info("Stuck transfer retried successfully", zap.String("id", t.ID), zap.Bool("skipped", skipped))
	}
}

// readinessLoop polls both chains until the engine has caught up.
func (e *Engine) readinessLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second) //nolint:mnd // 5 second readiness poll interval
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.checkReadiness(ctx)
			if e.IsReady() {
				e.logger.Info("Relayer is fully synced and ready")
				return
			}
		}
	}
}

// checkReadiness checks if both chains have caught up to their respective heads.
func (e *Engine) checkReadiness(ctx context.Context) {
	e.checkEthereumReadiness(ctx)
	e.checkCantonReadiness(ctx)
}

func (e *Engine) checkEthereumReadiness(ctx context.Context) {
	headBlock, err := e.ethClient.GetLatestBlockNumber(ctx)
	if err != nil {
		e.logger.Warn("Failed to get Ethereum latest block for readiness check", zap.Error(err))
		return
	}

	scannedBlock := e.ethClient.GetLastScannedBlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.ethereumSynced {
		return
	}

	lastProcessed := e.ethLastBlock
	if scannedBlock > lastProcessed {
		lastProcessed = scannedBlock
	}

	const lagTolerance = uint64(1)
	if lastProcessed+lagTolerance >= headBlock {
		e.ethereumSynced = true
		e.logger.Info("Ethereum processor initial sync complete",
			zap.Uint64("last_block", lastProcessed),
			zap.Uint64("head_block", headBlock))
	} else {
		e.logger.Debug("Ethereum processor catching up",
			zap.Uint64("last_block", lastProcessed),
			zap.Uint64("head_block", headBlock),
			zap.Uint64("blocks_behind", headBlock-lastProcessed))
	}
}

func (e *Engine) checkCantonReadiness(ctx context.Context) {
	ledgerEnd, err := e.cantonClient.GetLatestLedgerOffset(ctx)
	if err != nil {
		e.logger.Warn("Failed to get Canton ledger end for readiness check", zap.Error(err))
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cantonSynced {
		return
	}

	ledgerEndStr := strconv.FormatInt(ledgerEnd, 10)

	switch e.cantonOffset {
	case "", ledgerEndStr:
		e.cantonSynced = true
		e.logger.Info("Canton processor initial sync complete (at ledger end)", zap.String("offset", e.cantonOffset))
		return
	case relayer.OffsetBegin:
		// If starting from the beginning and the ledger is reachable, consider synced.
		e.cantonSynced = true
		e.logger.Info("Canton processor initial sync complete (BEGIN offset, ledger reachable)",
			zap.String("ledger_end", ledgerEndStr))
		return
	}

	currentOffset, err1 := strconv.ParseInt(e.cantonOffset, 10, 64)
	if err1 != nil {
		return
	}

	if currentOffset >= ledgerEnd {
		e.cantonSynced = true
		e.logger.Info("Canton processor initial sync complete",
			zap.String("offset", e.cantonOffset),
			zap.String("ledger_end", ledgerEndStr))
		return
	}

	// If the stream has been healthy long enough with no matching events, consider synced.
	const streamGracePeriod = 10 * time.Second
	if !e.cantonStreamStarted.IsZero() && time.Since(e.cantonStreamStarted) >= streamGracePeriod {
		e.cantonSynced = true
		e.logger.Info("Canton processor synced (stream healthy, no pending withdrawals)",
			zap.String("offset", e.cantonOffset),
			zap.String("ledger_end", ledgerEndStr))
		return
	}

	e.logger.Debug("Canton processor catching up",
		zap.String("offset", e.cantonOffset),
		zap.String("ledger_end", ledgerEndStr),
		zap.Int64("behind", ledgerEnd-currentOffset))
}
