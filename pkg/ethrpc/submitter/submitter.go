// Package submitter drains pending mempool entries by submitting the
// corresponding ERC-20 transfer to Canton. It is the asynchronous counterpart
// to ethrpc.service.SendRawTransaction: that handler records intent and
// returns the tx hash immediately, and this worker transitions each entry to
// completed (Canton accepted the transfer) or failed (Canton rejected it).
// The miner then seals the terminal entry into a synthetic EVM block, so
// eth_getTransactionReceipt returns a status=0x1 / 0x0 receipt as usual.
package submitter

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/token"

	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

const (
	// defaultConcurrency is the fallback when New is called with a non-positive
	// concurrency value, so misconfigurations don't silently disable the worker.
	defaultConcurrency = 10

	// cantonCallTimeout bounds a single Canton transfer call. Canton commits
	// typically land in 5-15s; 60s leaves generous headroom for the slow tail
	// while still ensuring a hung gRPC call can't park a worker slot
	// indefinitely. Deliberately not configurable — the value should be a
	// property of the Canton SLO, not per-deployment tuning.
	cantonCallTimeout = 60 * time.Second
)

// Store is the narrow data-access interface the submitter needs.
//
//go:generate mockery --name Store --output mocks --outpkg mocks --filename mock_store.go --with-expecter
type Store interface {
	// GetMempoolEntriesByStatus returns up to limit entries with the given
	// status, ordered by insertion ID. A limit of 0 means unlimited; the
	// submitter passes its batch size so a backlog never loads the entire
	// pending queue into memory.
	GetMempoolEntriesByStatus(ctx context.Context, status ethrpc.MempoolStatus, limit int) ([]ethrpc.MempoolEntry, error)
	CompleteMempoolEntry(ctx context.Context, txHash []byte) error
	FailMempoolEntry(ctx context.Context, txHash []byte, errMsg string) error
}

// TokenService is the narrow token-service interface needed for Canton transfers.
//
//go:generate mockery --name TokenService --output mocks --outpkg mocks --filename mock_token_service.go --with-expecter
type TokenService interface {
	ERC20(address common.Address) (token.ERC20, error)
}

// Submitter polls pending mempool entries and pushes them through Canton.
type Submitter struct {
	store       Store
	tokenSvc    TokenService
	interval    time.Duration
	batchSize   int
	concurrency int
	logger      *zap.Logger
}

// New creates a Submitter.
//
//   - interval is the tick spacing between drains.
//   - batchSize caps how many pending entries are fetched per tick (0 =
//     unlimited). Bounded so a backlog never loads the entire pending queue
//     into memory; the next tick picks up whatever is left.
//   - concurrency is the worker-pool width: how many Canton transfers run in
//     parallel within one tick (<= 0 defaults to defaultConcurrency so a
//     misconfiguration never silently disables the worker).
//
// The per-call Canton timeout (cantonCallTimeout) is fixed at package level
// — it's a property of the Canton SLO, not a per-deployment knob.
func New(
	store Store,
	tokenSvc TokenService,
	interval time.Duration,
	batchSize, concurrency int,
	logger *zap.Logger,
) *Submitter {
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	return &Submitter{
		store:       store,
		tokenSvc:    tokenSvc,
		interval:    interval,
		batchSize:   batchSize,
		concurrency: concurrency,
		logger:      logger,
	}
}

// Start runs the submitter loop until ctx is canceled.
func (s *Submitter) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.drain(ctx); err != nil {
				s.logger.Error("ethrpc submitter: drain failed", zap.Error(err))
			}
		}
	}
}

// drain processes a bounded batch of pending entries in parallel using a
// worker-pool of size s.concurrency. Each entry runs in its own goroutine with
// its own timed context (see process). drain returns only after every spawned
// goroutine finishes so two ticks never overlap on the same entry — the
// for-select loop in Start already serializes ticks, but drain doing wg.Wait()
// makes the contract explicit and survives any future refactor.
//
// On ctx cancellation: drain stops launching new goroutines, but lets in-flight
// ones finish (their child ctxs are derived from ctx, so they unwind quickly).
func (s *Submitter) drain(ctx context.Context) error {
	entries, err := s.store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolPending, s.batchSize)
	if err != nil {
		return fmt.Errorf("list pending mempool entries: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	// Buffered channel acts as a counting semaphore — at most s.concurrency
	// goroutines can hold a slot at once.
	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup
	for i := range entries {
		// Two-stage cancellation check: the explicit ctx.Err() up top makes
		// cancellation deterministic when both channels are ready (Go's
		// select would otherwise pick randomly); the select inside handles
		// the case where ctx is canceled while we're blocked on a saturated
		// pool.
		if ctx.Err() != nil {
			break
		}
		entry := &entries[i]
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.process(ctx, entry)
		}()
	}
	wg.Wait()
	return nil
}

// process submits a single pending entry to Canton, recording the outcome on
// the mempool row. Permanent (client-side) failures are recorded as failed so
// they reach the receipt; transient failures (network, gRPC unavailable) — and
// timeouts — leave the entry pending for retry on the next tick. Canton's
// command-id idempotency makes the retry safe.
//
// Each invocation runs under a per-entry timed context (cantonCallTimeout) so
// a single hung gRPC call cannot park a worker slot indefinitely. The same
// timed ctx is used for the mempool update too — if Canton commits at second
// 59.9 and the update can't complete before second 60, the entry simply stays
// pending and gets reconciled on the next tick (the Canton commit itself is
// preserved by Canton's idempotency contract).
func (s *Submitter) process(parent context.Context, entry *ethrpc.MempoolEntry) {
	ctx, cancel := context.WithTimeout(parent, cantonCallTimeout)
	defer cancel()

	contractAddr := common.HexToAddress(entry.ContractAddress)
	fromAddr := common.HexToAddress(entry.FromAddress)
	toAddr := common.HexToAddress(entry.RecipientAddress)
	amount := new(big.Int).SetBytes(entry.AmountData)
	txHash := common.BytesToHash(entry.TxHash)

	erc20, err := s.tokenSvc.ERC20(contractAddr)
	if err != nil {
		// Contract whitelist is validated synchronously in SendRawTransaction,
		// so reaching here means config drifted under us. Mark failed so the
		// client sees the error via the receipt rather than polling forever.
		s.failEntry(ctx, entry, fmt.Errorf("contract not supported: %w", err))
		return
	}

	transferErr := erc20.TransferFrom(ctx, txHash.Hex(), fromAddr, toAddr, *amount)
	if transferErr == nil {
		if completeErr := s.store.CompleteMempoolEntry(ctx, entry.TxHash); completeErr != nil {
			s.logger.Error("ethrpc submitter: complete mempool entry failed",
				zap.String("tx", txHash.Hex()),
				zap.Error(completeErr),
			)
		}
		return
	}

	if isPermanentError(transferErr) {
		s.failEntry(ctx, entry, transferErr)
		return
	}
	// Transient (network error, gRPC unavailable, ctx deadline exceeded):
	// leave as pending. Idempotent retry on the next tick.
	s.logger.Warn("ethrpc submitter: transient transfer failure, will retry",
		zap.String("tx", txHash.Hex()),
		zap.Error(transferErr),
	)
}

func (s *Submitter) failEntry(ctx context.Context, entry *ethrpc.MempoolEntry, cause error) {
	if updateErr := s.store.FailMempoolEntry(ctx, entry.TxHash, cause.Error()); updateErr != nil {
		s.logger.Error("ethrpc submitter: fail mempool entry update failed",
			zap.String("tx", common.BytesToHash(entry.TxHash).Hex()),
			zap.Error(updateErr),
		)
	}
}

// isPermanentError returns true when err is a categorized application error
// that would not benefit from retry — input validation, unsupported method,
// not-found, forbidden, conflict, gone. Dependency, recovering, timeout, and
// generic errors are treated as transient.
func isPermanentError(err error) bool {
	var svcErr *apperr.ServiceError
	if !errors.As(err, &svcErr) {
		return false
	}
	switch svcErr.Category {
	case apperr.CategoryDataError,
		apperr.CategoryNotSupported,
		apperr.CategoryUnauthorized,
		apperr.CategoryForbidden,
		apperr.CategoryResourceNotFound,
		apperr.CategoryDataConflict,
		apperr.CategoryGone:
		return true
	default:
		return false
	}
}
