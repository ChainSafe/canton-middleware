package submitter

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/chainsafe/canton-middleware/pkg/app/errors"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/submitter/mocks"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testFrom      = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testRecipient = "0xdddddddddddddddddddddddddddddddddddddddd"
	testContract  = "0xcccccccccccccccccccccccccccccccccccccccc"
)

func samplePendingEntry(txHash byte, amount int64) ethrpc.MempoolEntry {
	return ethrpc.MempoolEntry{
		ID:               int64(txHash),
		TxHash:           []byte{txHash},
		FromAddress:      testFrom,
		ContractAddress:  testContract,
		RecipientAddress: testRecipient,
		Nonce:            uint64(txHash),
		Input:            []byte{0xa9, 0x05, 0x9c, 0xbb},
		AmountData:       big.NewInt(amount).Bytes(),
		Status:           ethrpc.MempoolPending,
	}
}

func newTestSubmitter(store Store, tokenSvc TokenService) *Submitter {
	// Concurrency 1 keeps tests deterministic for assertions that depend on
	// ordering; tests that exercise the worker pool override New directly.
	return New(store, tokenSvc, 10*time.Millisecond, 0, 1, zap.NewNop())
}

// ─── drain() ─────────────────────────────────────────────────────────────────

func TestDrain_NoEntries(t *testing.T) {
	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, mock.Anything).
		Return(nil, nil)

	s := newTestSubmitter(store, mocks.NewTokenService(t))
	require.NoError(t, s.drain(context.Background()))
}

func TestDrain_GetEntriesError(t *testing.T) {
	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, mock.Anything).
		Return(nil, errors.New("db down"))

	s := newTestSubmitter(store, mocks.NewTokenService(t))
	err := s.drain(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

// ─── process(): success path ─────────────────────────────────────────────────

func TestProcess_Success_MarksCompleted(t *testing.T) {
	entry := samplePendingEntry(0x01, 42)

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, common.HexToAddress(testFrom), common.HexToAddress(testRecipient), mock.Anything).
		Return(nil)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)
	store.EXPECT().CompleteMempoolEntry(mock.Anything, entry.TxHash).Return(nil)

	s := newTestSubmitter(store, tokenSvc)
	s.process(context.Background(), &entry)
}

// ─── process(): permanent failure ────────────────────────────────────────────

func TestProcess_PermanentFailure_MarksFailed(t *testing.T) {
	entry := samplePendingEntry(0x02, 100)

	transferErr := apperr.BadRequestError(errors.New("user not found"), "failed to get sender")

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(transferErr)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)
	store.EXPECT().FailMempoolEntry(mock.Anything, entry.TxHash, mock.AnythingOfType("string")).Return(nil)

	s := newTestSubmitter(store, tokenSvc)
	s.process(context.Background(), &entry)
}

// ─── process(): transient failure ────────────────────────────────────────────

func TestProcess_TransientFailure_LeavesPending(t *testing.T) {
	entry := samplePendingEntry(0x03, 50)

	transferErr := apperr.DependencyError(errors.New("gRPC unavailable"), "canton transfer failed")

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(transferErr)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)
	// No Complete or Fail calls — store must not be touched on transient errors.

	s := newTestSubmitter(store, tokenSvc)
	s.process(context.Background(), &entry)

	// Cross-check by AssertExpectations (called via NewStore cleanup).
	store.AssertNotCalled(t, "CompleteMempoolEntry", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "FailMempoolEntry", mock.Anything, mock.Anything, mock.Anything)
}

// Uncategorized error must also be treated as transient — the contract is
// "permanent only if explicitly categorized as a client-side problem".
func TestProcess_UncategorizedError_LeavesPending(t *testing.T) {
	entry := samplePendingEntry(0x04, 75)

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("connection refused"))

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)

	s := newTestSubmitter(store, tokenSvc)
	s.process(context.Background(), &entry)

	store.AssertNotCalled(t, "CompleteMempoolEntry", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "FailMempoolEntry", mock.Anything, mock.Anything, mock.Anything)
}

// ─── process(): contract whitelist drift ─────────────────────────────────────

func TestProcess_ContractNotSupported_MarksFailed(t *testing.T) {
	entry := samplePendingEntry(0x05, 1)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(nil, errors.New("unknown token"))

	store := mocks.NewStore(t)
	store.EXPECT().FailMempoolEntry(mock.Anything, entry.TxHash, mock.MatchedBy(func(msg string) bool {
		return msg != ""
	})).Return(nil)

	s := newTestSubmitter(store, tokenSvc)
	s.process(context.Background(), &entry)
}

// ─── drain(): batch size is pushed to the store as the SQL limit ─────────────

func TestDrain_BatchSizePushedToStore(t *testing.T) {
	// Submitter must forward its batch size to the store so the LIMIT is
	// applied in SQL (preventing a backlog from being loaded into memory).
	const batchSize = 2

	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, batchSize).
		Return([]ethrpc.MempoolEntry{
			samplePendingEntry(0x01, 1),
			samplePendingEntry(0x02, 2),
		}, nil)
	store.EXPECT().CompleteMempoolEntry(mock.Anything, mock.Anything).Return(nil).Times(2)

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Times(2)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(mock.Anything).Return(erc20, nil).Times(2)

	s := New(store, tokenSvc, time.Second, batchSize, 1, zap.NewNop())
	require.NoError(t, s.drain(context.Background()))
}

// ─── drain(): ctx cancellation stops mid-batch ───────────────────────────────

func TestDrain_ContextCanceledMidBatch(t *testing.T) {
	entries := []ethrpc.MempoolEntry{
		samplePendingEntry(0x01, 1),
		samplePendingEntry(0x02, 2),
	}

	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, mock.Anything).Return(entries, nil)

	tokenSvc := mocks.NewTokenService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	s := newTestSubmitter(store, tokenSvc)
	require.NoError(t, s.drain(ctx))

	// No ERC20 lookups should occur because the loop exits before processing.
	tokenSvc.AssertNotCalled(t, "ERC20", mock.Anything)
}

// ─── Start() lifecycle ───────────────────────────────────────────────────────

func TestStart_StopsOnContextCancel(t *testing.T) {
	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, mock.Anything).Return(nil, nil).Maybe()

	tokenSvc := mocks.NewTokenService(t)

	s := New(store, tokenSvc, 5*time.Millisecond, 0, 1, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Start returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// ─── concurrency ─────────────────────────────────────────────────────────────

// TestDrain_RunsEntriesConcurrently verifies that the worker pool actually
// processes entries in parallel. We block each TransferFrom on a barrier that
// only releases once we observe `concurrency` goroutines in flight at the same
// time — if drain were sequential, the test would deadlock and time out.
func TestDrain_RunsEntriesConcurrently(t *testing.T) {
	const concurrency = 3
	entries := make([]ethrpc.MempoolEntry, concurrency)
	for i := range entries {
		entries[i] = samplePendingEntry(byte(i+1), int64(i+1))
	}

	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, mock.Anything).Return(entries, nil)
	store.EXPECT().CompleteMempoolEntry(mock.Anything, mock.Anything).Return(nil).Times(concurrency)

	// Counts goroutines that have entered TransferFrom; release() unblocks
	// them once we've seen all `concurrency` arrive simultaneously.
	var inFlight int32
	allArrived := make(chan struct{})
	release := make(chan struct{})
	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _, _ common.Address, _ big.Int) error {
			if atomic.AddInt32(&inFlight, 1) == int32(concurrency) {
				close(allArrived)
			}
			<-release
			return nil
		}).Times(concurrency)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(mock.Anything).Return(erc20, nil).Times(concurrency)

	s := New(store, tokenSvc, time.Second, 0, concurrency, zap.NewNop())

	drained := make(chan error, 1)
	go func() { drained <- s.drain(context.Background()) }()

	// All `concurrency` workers must reach TransferFrom before any returns.
	select {
	case <-allArrived:
	case <-time.After(2 * time.Second):
		t.Fatalf("only %d goroutines reached TransferFrom; pool is not concurrent",
			atomic.LoadInt32(&inFlight))
	}
	close(release)

	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("drain did not return after releasing workers")
	}
}

// TestDrain_RespectsConcurrencyCap verifies that no more than `concurrency`
// TransferFrom calls run simultaneously even when the batch has more entries
// than the pool size. Track peak in-flight count and assert it never exceeds
// the cap.
func TestDrain_RespectsConcurrencyCap(t *testing.T) {
	const (
		concurrency = 2
		batch       = 6
	)
	entries := make([]ethrpc.MempoolEntry, batch)
	for i := range entries {
		entries[i] = samplePendingEntry(byte(i+1), int64(i+1))
	}

	store := mocks.NewStore(t)
	store.EXPECT().GetMempoolEntriesByStatus(mock.Anything, ethrpc.MempoolPending, mock.Anything).Return(entries, nil)
	store.EXPECT().CompleteMempoolEntry(mock.Anything, mock.Anything).Return(nil).Times(batch)

	var (
		inFlight int32
		peak     int32
	)
	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _, _ common.Address, _ big.Int) error {
			cur := atomic.AddInt32(&inFlight, 1)
			// Track the high-water mark of concurrent workers.
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
			return nil
		}).Times(batch)

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(mock.Anything).Return(erc20, nil).Times(batch)

	s := New(store, tokenSvc, time.Second, 0, concurrency, zap.NewNop())
	require.NoError(t, s.drain(context.Background()))

	require.LessOrEqual(t, int(atomic.LoadInt32(&peak)), concurrency,
		"peak concurrent workers exceeded the cap")
}

// New() must coerce a non-positive concurrency to the package default rather
// than silently disabling the pool — a zero-buffered semaphore would deadlock.
func TestNew_NonPositiveConcurrencyDefaults(t *testing.T) {
	s := New(nil, nil, time.Second, 0, 0, zap.NewNop())
	require.Equal(t, defaultConcurrency, s.concurrency)

	s = New(nil, nil, time.Second, 0, -5, zap.NewNop())
	require.Equal(t, defaultConcurrency, s.concurrency)
}

// ─── per-process timed context ───────────────────────────────────────────────

// TestProcess_CantonContextDone_LeavesPending verifies that a Canton call
// returning ctx-derived cancellation is treated as transient — entry stays
// pending so the next tick retries. The hardcoded cantonCallTimeout is too
// long to wait on in tests, but its derived ctx still inherits cancellation
// from the parent, so we cancel the parent and let propagation drive the
// timeout path with no real delay.
func TestProcess_CantonContextDone_LeavesPending(t *testing.T) {
	entry := samplePendingEntry(0x06, 1)

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ string, _, _ common.Address, _ big.Int) error {
			// Block until the per-process ctx fires (propagated from parent).
			<-ctx.Done()
			return ctx.Err()
		})

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	// Fire cancellation after a brief moment so TransferFrom is definitely
	// blocked on <-ctx.Done() rather than racing with the cancel.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	s := New(store, tokenSvc, time.Second, 0, 1, zap.NewNop())
	s.process(ctx, &entry)

	// Cancellation/timeout is transient → no Complete/Fail call.
	store.AssertNotCalled(t, "CompleteMempoolEntry", mock.Anything, mock.Anything)
	store.AssertNotCalled(t, "FailMempoolEntry", mock.Anything, mock.Anything, mock.Anything)
}

// TestProcess_DBWriteUsesFreshContext_OnSuccess proves the mempool status
// update is *not* tied to the Canton-scoped ctx — otherwise a Canton call
// that nearly exhausts its 60s budget would leave no room to write the
// outcome, and a permanently-failing entry would loop forever. We assert
// CompleteMempoolEntry lands with a non-expired ctx whose deadline matches
// dbWriteTimeout (within tolerance), even when TransferFrom has just observed
// its own ctx fire.
func TestProcess_DBWriteUsesFreshContext_OnSuccess(t *testing.T) {
	entry := samplePendingEntry(0x07, 1)

	// Capture the Canton ctx so we can confirm the DB ctx is a different one.
	var cantonCtx context.Context
	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ string, _, _ common.Address, _ big.Int) error {
			cantonCtx = ctx
			return nil
		})

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)
	store.EXPECT().
		CompleteMempoolEntry(mock.MatchedBy(func(ctx context.Context) bool {
			if ctx == cantonCtx { // must be a freshly-derived ctx, not the Canton one
				return false
			}
			if err := ctx.Err(); err != nil {
				return false
			}
			deadline, ok := ctx.Deadline()
			// dbWriteTimeout = 10s; allow a generous lower bound so test
			// timing jitter doesn't flake the assertion.
			return ok && time.Until(deadline) > 5*time.Second
		}), entry.TxHash).
		Return(nil)

	s := New(store, tokenSvc, time.Second, 0, 1, zap.NewNop())
	s.process(context.Background(), &entry)
}

// TestFailEntry_UsesFreshContext mirrors the success path: a permanent error
// must record `failed` under its own dbWriteTimeout-bounded ctx, not the
// Canton ctx. This is the bug Gemini flagged: if FailMempoolEntry inherits
// an expired Canton ctx, the entry stays pending and the submitter retries
// the permanently-failing transaction forever.
func TestFailEntry_UsesFreshContext(t *testing.T) {
	entry := samplePendingEntry(0x08, 1)

	erc20 := mocks.NewERC20(t)
	erc20.EXPECT().
		TransferFrom(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(apperr.BadRequestError(errors.New("nope"), "permanent"))

	tokenSvc := mocks.NewTokenService(t)
	tokenSvc.EXPECT().ERC20(common.HexToAddress(testContract)).Return(erc20, nil)

	store := mocks.NewStore(t)
	store.EXPECT().
		FailMempoolEntry(mock.MatchedBy(func(ctx context.Context) bool {
			if err := ctx.Err(); err != nil {
				return false
			}
			deadline, ok := ctx.Deadline()
			return ok && time.Until(deadline) > 5*time.Second
		}), entry.TxHash, mock.AnythingOfType("string")).
		Return(nil)

	s := New(store, tokenSvc, time.Second, 0, 1, zap.NewNop())
	s.process(context.Background(), &entry)
}

// ─── isPermanentError ────────────────────────────────────────────────────────

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not permanent", nil, false},
		{"plain error is transient", errors.New("oops"), false},
		{"BadRequest is permanent", apperr.BadRequestError(errors.New(""), "x"), true},
		{"NotSupported is permanent", apperr.NotSupportedError(errors.New(""), "x"), true},
		{"Forbidden is permanent", apperr.ForbiddenError(errors.New(""), "x"), true},
		{"NotFound is permanent", apperr.ResourceNotFoundError(errors.New(""), "x"), true},
		{"Conflict is permanent", apperr.ConflictError(errors.New(""), "x"), true},
		{"Gone is permanent", apperr.GoneError(errors.New(""), "x"), true},
		{"Dependency is transient", apperr.DependencyError(errors.New(""), "x"), false},
		{"General is transient", apperr.GeneralError(errors.New("x")), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isPermanentError(tc.err))
		})
	}
}
