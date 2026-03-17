package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/indexer/engine/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Test event / batch builders (reuse constants from decoder_test.go)
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

func transferEventParsed() *indexer.ParsedEvent {
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

func makeProcBatch(offset int64, events ...*indexer.ParsedEvent) *streaming.Batch[*indexer.ParsedEvent] {
	return &streaming.Batch[*indexer.ParsedEvent]{
		Offset:   offset,
		UpdateID: "update-" + string(rune('0'+offset)),
		Items:    events,
	}
}

// feedCh sends batches into a buffered channel and closes it.
func feedCh(batches ...*streaming.Batch[*indexer.ParsedEvent]) <-chan *streaming.Batch[*indexer.ParsedEvent] {
	ch := make(chan *streaming.Batch[*indexer.ParsedEvent], len(batches))
	for _, b := range batches {
		ch <- b
	}
	close(ch)
	return ch
}

// setupRunInTx wires RunInTx to immediately execute its callback with the mock store.
func setupRunInTx(store *mocks.Store) {
	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, indexer.Store) error) error {
			return fn(ctx, store)
		})
}

// ---------------------------------------------------------------------------
// tokenFromEvent
// ---------------------------------------------------------------------------

func TestTokenFromEvent(t *testing.T) {
	e := mintEvent()
	tok := tokenFromEvent(e)

	assert.Equal(t, testInstrumentAdmin, tok.InstrumentAdmin)
	assert.Equal(t, testInstrumentID, tok.InstrumentID)
	assert.Equal(t, testIssuer, tok.Issuer)
	assert.Equal(t, int64(1), tok.FirstSeenOffset)
	assert.Equal(t, time.Unix(1_700_000_000, 0), tok.FirstSeenAt)
	// TotalSupply and HolderCount are left at zero — the store maintains them.
	assert.Empty(t, tok.TotalSupply)
	assert.Equal(t, int64(0), tok.HolderCount)
}

// ---------------------------------------------------------------------------
// supplyDeltaFromEvent
// ---------------------------------------------------------------------------

func TestSupplyDeltaFromEvent_Mint(t *testing.T) {
	_, _, delta, ok := supplyDeltaFromEvent(mintEvent())
	require.True(t, ok)
	assert.Equal(t, testAmount, delta)
}

func TestSupplyDeltaFromEvent_Burn(t *testing.T) {
	_, _, delta, ok := supplyDeltaFromEvent(burnEvent())
	require.True(t, ok)
	assert.Equal(t, "-"+testAmount, delta)
}

func TestSupplyDeltaFromEvent_Transfer_NoOp(t *testing.T) {
	instrumentAdmin, instrumentID, delta, ok := supplyDeltaFromEvent(transferEventParsed())
	assert.Empty(t, instrumentAdmin)
	assert.Empty(t, instrumentID)
	assert.Empty(t, delta)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// balanceUpdatesFromEvent
// ---------------------------------------------------------------------------

func TestBalanceUpdatesFromEvent_Mint(t *testing.T) {
	updates := balanceUpdatesFromEvent(mintEvent())
	require.Len(t, updates, 1)
	assert.Equal(t, testRecipient, updates[0][0])
	assert.Equal(t, testAmount, updates[0][1])
}

func TestBalanceUpdatesFromEvent_Burn(t *testing.T) {
	updates := balanceUpdatesFromEvent(burnEvent())
	require.Len(t, updates, 1)
	assert.Equal(t, testSender, updates[0][0])
	assert.Equal(t, "-"+testAmount, updates[0][1])
}

func TestBalanceUpdatesFromEvent_Transfer(t *testing.T) {
	updates := balanceUpdatesFromEvent(transferEventParsed())
	require.Len(t, updates, 2)
	assert.Equal(t, testSender, updates[0][0])
	assert.Equal(t, "-"+testAmount, updates[0][1])
	assert.Equal(t, testRecipient, updates[1][0])
	assert.Equal(t, testAmount, updates[1][1])
}

// ---------------------------------------------------------------------------
// Processor.Run: startup / lifecycle
// ---------------------------------------------------------------------------

func TestProcessor_Run_LoadOffsetError(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	loadErr := errors.New("db down")

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), loadErr)

	p := NewProcessor(fetcher, store, zap.NewNop())
	err := p.Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, loadErr)
}

func TestProcessor_Run_StreamClosed_ReturnsNil(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(5), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(5))
	fetcher.EXPECT().Events().Return(feedCh())

	p := NewProcessor(fetcher, store, zap.NewNop())
	assert.NoError(t, p.Run(context.Background()))
}

func TestProcessor_Run_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	ch := make(chan *streaming.Batch[*indexer.ParsedEvent]) // never closed / sent

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return((<-chan *streaming.Batch[*indexer.ParsedEvent])(ch))

	p := NewProcessor(fetcher, store, zap.NewNop())

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

// ---------------------------------------------------------------------------
// Processor.Run: per-event-type store call verification
// ---------------------------------------------------------------------------

func TestProcessor_Run_MintBatch(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := mintEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeProcBatch(1, ev)))

	setupRunInTx(store)
	store.EXPECT().UpsertToken(mock.Anything, tokenFromEvent(ev)).Return(nil)
	store.EXPECT().ApplySupplyDelta(mock.Anything, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testRecipient, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().SaveBatch(mock.Anything, int64(1), []*indexer.ParsedEvent{ev}).Return(nil)

	require.NoError(t, NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_BurnBatch(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := burnEvent()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeProcBatch(2, ev)))

	setupRunInTx(store)
	store.EXPECT().UpsertToken(mock.Anything, tokenFromEvent(ev)).Return(nil)
	store.EXPECT().ApplySupplyDelta(mock.Anything, testInstrumentAdmin, testInstrumentID, "-"+testAmount).Return(nil)
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testSender, testInstrumentAdmin, testInstrumentID, "-"+testAmount).Return(nil)
	store.EXPECT().SaveBatch(mock.Anything, int64(2), []*indexer.ParsedEvent{ev}).Return(nil)

	require.NoError(t, NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_TransferBatch(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)
	ev := transferEventParsed()

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeProcBatch(3, ev)))

	setupRunInTx(store)
	store.EXPECT().UpsertToken(mock.Anything, tokenFromEvent(ev)).Return(nil)
	// Transfer: no supply delta.
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testSender, testInstrumentAdmin, testInstrumentID, "-"+testAmount).Return(nil)
	store.EXPECT().ApplyBalanceDelta(mock.Anything, testRecipient, testInstrumentAdmin, testInstrumentID, testAmount).Return(nil)
	store.EXPECT().SaveBatch(mock.Anything, int64(3), []*indexer.ParsedEvent{ev}).Return(nil)

	require.NoError(t, NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_EmptyBatch_AdvancesOffset(t *testing.T) {
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(9), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(9))
	fetcher.EXPECT().Events().Return(feedCh(makeProcBatch(10)))

	setupRunInTx(store)
	// No UpsertToken, ApplySupplyDelta, or ApplyBalanceDelta calls.
	store.EXPECT().SaveBatch(mock.Anything, int64(10), ([]*indexer.ParsedEvent)(nil)).Return(nil)

	require.NoError(t, NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

// ---------------------------------------------------------------------------
// Processor.Run: retry on transient store error
// ---------------------------------------------------------------------------

func TestProcessor_Run_ProcessBatch_StoreError_Retries(t *testing.T) {
	processorRetryBaseDelay = time.Millisecond
	defer func() { processorRetryBaseDelay = 5 * time.Second }()

	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeProcBatch(1)))

	// First attempt fails.
	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		Return(errors.New("transient db error")).Once()
	// Second attempt succeeds.
	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context, indexer.Store) error) error {
			return fn(ctx, store)
		}).Once()
	store.EXPECT().SaveBatch(mock.Anything, int64(1), ([]*indexer.ParsedEvent)(nil)).Return(nil)

	require.NoError(t, NewProcessor(fetcher, store, zap.NewNop()).Run(context.Background()))
}

func TestProcessor_Run_ContextCancelledDuringRetry(t *testing.T) {
	processorRetryBaseDelay = time.Hour // effectively infinite
	defer func() { processorRetryBaseDelay = 5 * time.Second }()

	ctx, cancel := context.WithCancel(context.Background())
	store := mocks.NewStore(t)
	fetcher := mocks.NewEventFetcher(t)

	store.EXPECT().LatestOffset(mock.Anything).Return(int64(0), nil)
	fetcher.EXPECT().Start(mock.Anything, int64(0))
	fetcher.EXPECT().Events().Return(feedCh(makeProcBatch(1)))

	// Always fail; cancel immediately so the retry wait is interrupted.
	store.EXPECT().RunInTx(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ func(context.Context, indexer.Store) error) error {
			cancel()
			return errors.New("persistent db error")
		})

	err := NewProcessor(fetcher, store, zap.NewNop()).Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
