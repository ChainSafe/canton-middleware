// SPDX-License-Identifier: Apache-2.0

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

	// dbWriteTimeout bounds the mempool-status update that follows a Canton
	// call. It's a fresh deadline derived from the drain ctx (not the Canton
	// ctx) so a Canton call that just barely beats its 60s budget still has
	// room to record the outcome — otherwise a permanent failure could leave
	// the entry pending and the submitter would retry against a Canton that
	// already rejected (or, worse, already accepted) the transfer.
	dbWriteTimeout = 10 * time.Second
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
	metrics     *Metrics
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
//   - metrics receives Prometheus observations from the drain and per-entry
//     paths. Tests can pass NewNopMetrics() to ignore them.
//
// The per-call Canton timeout (cantonCallTimeout) is fixed at package level
// — it's a property of the Canton SLO, not a per-deployment knob.
func New(
	store Store,
	tokenSvc TokenService,
	interval time.Duration,
	batchSize, concurrency int,
	metrics *Metrics,
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
		metrics:     metrics,
		logger:      logger,
	}
}

// Start runs the submitter loop until ctx is canceled.
func (s *Submitter) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
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
	start := time.Now()
	defer func() {
		s.metrics.DrainDuration.Observe(time.Since(start).Seconds())
	}()

	entries, err := s.store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolPending, s.batchSize)
	if err != nil {
		s.metrics.DrainErrorsTotal.Inc()
		return fmt.Errorf("list pending mempool entries: %w", err)
	}

	s.metrics.EntriesFetched.Observe(float64(len(entries)))
	s.metrics.PendingBacklog.Set(float64(len(entries)))

	if len(entries) == 0 {
		s.metrics.LastSuccessfulDrainTimestamp.SetToCurrentTime()
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
		// Non-blocking acquire first so we can count saturation events
		// (worker_pool_blocked_total) separately from the normal fast path.
		// A non-zero counter here means concurrency is undersized for the
		// arrival rate.
		select {
		case sem <- struct{}{}:
		default:
			s.metrics.WorkerPoolBlockedTotal.Inc()
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			case sem <- struct{}{}:
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.process(ctx, entry)
		}()
	}
	wg.Wait()
	s.metrics.LastSuccessfulDrainTimestamp.SetToCurrentTime()
	return nil
}

// process submits a single pending entry to Canton, recording the outcome on
// the mempool row. Permanent (client-side) failures are recorded as failed so
// they reach the receipt; transient failures (network, gRPC unavailable) — and
// timeouts — leave the entry pending for retry on the next tick. Canton's
// command-id idempotency makes the retry safe.
//
// The Canton call runs under its own cantonCallTimeout deadline so a hung gRPC
// call can't park a worker slot indefinitely. The follow-up mempool-status
// write is intentionally derived from parent (not from the Canton ctx) with
// its own dbWriteTimeout — otherwise a Canton call that nearly exhausts its
// 60s budget would leave no time for the DB update, the row would stay
// pending, and a permanent failure would loop forever against a Canton that
// already rejected it.
func (s *Submitter) process(parent context.Context, entry *ethrpc.MempoolEntry) {
	start := time.Now()
	// Default to transient_retry — if process returns through any path without
	// reaching a terminal branch (e.g. an unexpected panic in the future),
	// "leave pending" is the safer classification.
	outcome := "transient_retry"
	defer func() {
		s.metrics.EntriesProcessedTotal.WithLabelValues(outcome).Inc()
		s.metrics.EntryProcessDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
	}()

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
		s.failEntry(parent, entry, fmt.Errorf("contract not supported: %w", err))
		outcome = "failed_permanent"
		return
	}

	cantonStart := time.Now()
	cantonCtx, cancel := context.WithTimeout(parent, cantonCallTimeout)
	defer cancel()
	transferErr := erc20.TransferFrom(cantonCtx, txHash.Hex(), fromAddr, toAddr, *amount)
	cantonDur := time.Since(cantonStart).Seconds()

	if transferErr == nil {
		s.metrics.CantonTransferDuration.WithLabelValues("ok").Observe(cantonDur)
		s.completeEntry(parent, entry, txHash)
		outcome = "completed"
		return
	}

	s.metrics.CantonTransferErrorsTotal.WithLabelValues(categorizeTransferError(transferErr)).Inc()

	if isPermanentError(transferErr) {
		s.metrics.CantonTransferDuration.WithLabelValues("permanent").Observe(cantonDur)
		s.failEntry(parent, entry, transferErr)
		outcome = "failed_permanent"
		return
	}

	// Transient (network error, gRPC unavailable, ctx deadline exceeded):
	// leave as pending. Idempotent retry on the next tick.
	s.metrics.CantonTransferDuration.WithLabelValues("transient").Observe(cantonDur)
	s.logger.Warn("ethrpc submitter: transient transfer failure, will retry",
		zap.String("tx", txHash.Hex()),
		zap.Error(transferErr),
	)
}

// completeEntry writes the pending → completed transition under its own short
// deadline derived from parent (see dbWriteTimeout doc).
func (s *Submitter) completeEntry(parent context.Context, entry *ethrpc.MempoolEntry, txHash common.Hash) {
	ctx, cancel := context.WithTimeout(parent, dbWriteTimeout)
	defer cancel()
	if err := s.store.CompleteMempoolEntry(ctx, entry.TxHash); err != nil {
		s.metrics.DBWriteErrorsTotal.WithLabelValues("complete").Inc()
		s.logger.Error("ethrpc submitter: complete mempool entry failed",
			zap.String("tx", txHash.Hex()),
			zap.Error(err),
		)
	}
}

// failEntry writes the pending → failed transition under its own short
// deadline derived from parent (see dbWriteTimeout doc).
func (s *Submitter) failEntry(parent context.Context, entry *ethrpc.MempoolEntry, cause error) {
	ctx, cancel := context.WithTimeout(parent, dbWriteTimeout)
	defer cancel()
	if err := s.store.FailMempoolEntry(ctx, entry.TxHash, cause.Error()); err != nil {
		s.metrics.DBWriteErrorsTotal.WithLabelValues("fail").Inc()
		s.logger.Error("ethrpc submitter: fail mempool entry update failed",
			zap.String("tx", common.BytesToHash(entry.TxHash).Hex()),
			zap.Error(err),
		)
	}
}

// categorizeTransferError maps a Canton TransferFrom error to a stable label
// value for canton_transfer_errors_total. The taxonomy mirrors apperr.Category
// (lowercased without the "Category" prefix) plus a "timeout" / "transient"
// fallback for cases that don't carry a *apperr.ServiceError. Keep this in
// sync with isPermanentError — the permanent categories there must map to
// labels distinct from "transient".
func categorizeTransferError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var svcErr *apperr.ServiceError
	if !errors.As(err, &svcErr) {
		return "transient"
	}
	switch svcErr.Category {
	case apperr.CategoryDataError:
		return "data"
	case apperr.CategoryNotSupported:
		return "not_supported"
	case apperr.CategoryUnauthorized:
		return "unauthorized"
	case apperr.CategoryForbidden:
		return "forbidden"
	case apperr.CategoryResourceNotFound:
		return "not_found"
	case apperr.CategoryDataConflict:
		return "conflict"
	case apperr.CategoryGone:
		return "gone"
	case apperr.CategoryConnectionTimeout:
		return "timeout"
	default:
		return "transient"
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
