package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/indexer/engine"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine/mocks"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testInstrumentID    = "DEMO"
	testInstrumentAdmin = "issuer-party::abc123"
	testIssuer          = "issuer-party::abc123"
	testAmount          = "100.000000000000000000"
	testRecipient       = "recipient-party::def456"
	testSender          = "sender-party::ghi789"
	testContractID      = "contract-id-1"
)

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

func mintEvent() *indexer.ParsedEvent {
	r := testRecipient
	return &indexer.ParsedEvent{
		EventType:       indexer.EventMint,
		InstrumentID:    testInstrumentID,
		InstrumentAdmin: testInstrumentAdmin,
		Issuer:          testIssuer,
		Amount:          testAmount,
		ToPartyID:       &r,
		ContractID:      testContractID,
		LedgerOffset:    1,
		EffectiveTime:   time.Unix(1_700_000_000, 0),
	}
}

func burnEvent() *indexer.ParsedEvent {
	s := testSender
	return &indexer.ParsedEvent{
		EventType:       indexer.EventBurn,
		InstrumentID:    testInstrumentID,
		InstrumentAdmin: testInstrumentAdmin,
		Issuer:          testIssuer,
		Amount:          testAmount,
		FromPartyID:     &s,
		ContractID:      testContractID,
		LedgerOffset:    2,
		EffectiveTime:   time.Unix(1_700_000_000, 0),
	}
}

func transferEvent() *indexer.ParsedEvent {
	s := testSender
	r := testRecipient
	return &indexer.ParsedEvent{
		EventType:       indexer.EventTransfer,
		InstrumentID:    testInstrumentID,
		InstrumentAdmin: testInstrumentAdmin,
		Issuer:          testIssuer,
		Amount:          testAmount,
		FromPartyID:     &s,
		ToPartyID:       &r,
		ContractID:      testContractID,
		LedgerOffset:    3,
		EffectiveTime:   time.Unix(1_700_000_000, 0),
	}
}

func makeBatch(offset int64, events ...*indexer.ParsedEvent) *streaming.Batch[*indexer.ParsedEvent] {
	return &streaming.Batch[*indexer.ParsedEvent]{
		Offset:   offset,
		UpdateID: "update-" + string(rune('0'+offset)),
		Items:    events,
	}
}

func feedCh(batches ...*streaming.Batch[*indexer.ParsedEvent]) <-chan *streaming.Batch[*indexer.ParsedEvent] {
	ch := make(chan *streaming.Batch[*indexer.ParsedEvent], len(batches))
	for _, b := range batches {
		ch <- b
	}
	close(ch)
	return ch
}

func setupRunInTx(store *mocks.Store) {
	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, engine.Store) error) error {
			return fn(ctx, store)
		})
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func TestProcessor_Run_LoadOffsetError(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	loadErr := errors.New("db down")

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), loadErr)

	err := engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, loadErr)
}

func TestProcessor_Run_StreamClosed_ReturnsNil(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(5), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(5))
	fetcher.EXPECT().Events().Return(feedCh())

	assert.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	ch := make(chan *streaming.Batch[*indexer.ParsedEvent])

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return((<-chan *streaming.Batch[*indexer.ParsedEvent])(ch))

	done := make(chan error, 1)
	go func() { done <- engine.NewProcessor(fetcher, store, zap.NewNop()).Run(ctx) }()

	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

// ---------------------------------------------------------------------------
// Event-type store call verification
// (also implicitly tests tokenFromEvent / supplyDeltaFromEvent / balanceUpdatesFromEvent)
// ---------------------------------------------------------------------------

func TestProcessor_Run_MintBatch(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := mintEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(1, ev)))

	setupRunInTx(store)
	store.EXPECT().InsertEvent(mock.Anything, ev).Return(true, nil)
	store.EXPECT().UpsertToken(mock.Anything, &indexer.Token{
		InstrumentAdmin: testInstrumentAdmin,
		InstrumentID:    testInstrumentID,
		Issuer:          testIssuer,
		FirstSeenOffset: 1,
		FirstSeenAt:     time.Unix(1_700_000_000, 0),
	}).Return(nil)
	store.EXPECT().ApplySupplyDelta(mock.Anything, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testRecipient, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().SaveOffset(mock.Anything, int64(1)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_BurnBatch(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := burnEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(2, ev)))

	setupRunInTx(store)
	store.EXPECT().InsertEvent(mock.Anything, ev).Return(true, nil)
	store.EXPECT().UpsertToken(mock.Anything, &indexer.Token{
		InstrumentAdmin: testInstrumentAdmin,
		InstrumentID:    testInstrumentID,
		Issuer:          testIssuer,
		FirstSeenOffset: 2,
		FirstSeenAt:     time.Unix(1_700_000_000, 0),
	}).Return(nil)
	store.EXPECT().ApplySupplyDelta(mock.Anything, testInstrumentAdmin, testInstrumentID, "-"+testAmount).Return(nil)
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testSender, testInstrumentAdmin, testInstrumentID, "-"+testAmount).Return(nil)
	store.EXPECT().SaveOffset(mock.Anything, int64(2)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_TransferBatch(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := transferEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(3, ev)))

	setupRunInTx(store)
	store.EXPECT().InsertEvent(mock.Anything, ev).Return(true, nil)
	store.EXPECT().UpsertToken(mock.Anything, &indexer.Token{
		InstrumentAdmin: testInstrumentAdmin,
		InstrumentID:    testInstrumentID,
		Issuer:          testIssuer,
		FirstSeenOffset: 3,
		FirstSeenAt:     time.Unix(1_700_000_000, 0),
	}).Return(nil)
	// Transfer: no ApplySupplyDelta.
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testSender, testInstrumentAdmin, testInstrumentID, "-"+testAmount).Return(nil)
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testRecipient, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().SaveOffset(mock.Anything, int64(3)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

// TestProcessor_Run_Transfer_CrossParticipantSender verifies that when the sender's
// ApplyBalanceDelta returns ErrNegativeBalance (their mint history lives on another
// participant and was never delivered to this one), the processor skips the sender
// deduction, still applies the receiver credit, and commits the offset.
func TestProcessor_Run_Transfer_CrossParticipantSender(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := transferEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(3, ev)))

	setupRunInTx(store)
	store.EXPECT().InsertEvent(mock.Anything, ev).Return(true, nil)
	store.EXPECT().UpsertToken(mock.Anything, &indexer.Token{
		InstrumentAdmin: testInstrumentAdmin,
		InstrumentID:    testInstrumentID,
		Issuer:          testIssuer,
		FirstSeenOffset: 3,
		FirstSeenAt:     time.Unix(1_700_000_000, 0),
	}).Return(nil)
	// Sender delta returns ErrNegativeBalance — simulates a P2-only party with no local mint history.
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testSender, testInstrumentAdmin, testInstrumentID, "-"+testAmount).
		Return(fmt.Errorf("%w for party %s: current=0 delta=-%s", engine.ErrNegativeBalance, testSender, testAmount))
	// Receiver credit must still be applied despite the sender error being skipped.
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testRecipient, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().SaveOffset(mock.Anything, int64(3)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_EmptyBatch_AdvancesOffset(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(9), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(9))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(10)))

	setupRunInTx(store)
	store.EXPECT().SaveOffset(mock.Anything, int64(10)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_DuplicateEvent_SkipsDerivedStateButAdvancesOffset(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := mintEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(4), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(4))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(5, ev)))

	setupRunInTx(store)
	store.EXPECT().InsertEvent(mock.Anything, ev).Return(false, nil)
	store.EXPECT().SaveOffset(mock.Anything, int64(5)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

// ---------------------------------------------------------------------------
// Retry behavior
// ---------------------------------------------------------------------------

func TestProcessor_Run_StoreError_Retries(t *testing.T) {
	engine.SetRetryBaseDelay(t, time.Millisecond)

	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(1)))

	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		Return(errors.New("transient error")).Once()
	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, engine.Store) error) error {
			return fn(ctx, store)
		}).Once()
	store.EXPECT().SaveOffset(mock.Anything, int64(1)).Return(nil)

	require.NoError(t, engine.NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_ContextCancelledDuringRetry(t *testing.T) {
	engine.SetRetryBaseDelay(t, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeBatch(1)))

	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ func(context.Context, engine.Store) error) error {
			cancel()
			return errors.New("persistent error")
		})

	err := engine.NewProcessor(fetcher, store, zap.NewNop()).Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
