// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
	relayermocks "github.com/chainsafe/canton-middleware/pkg/relayer/engine/mocks"
)

// fakeBridge is a hand-rolled TokenBridge whose Step behavior is set per test.
type fakeBridge struct {
	key     string
	sources []relayer.Source
	stepFn  func(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error)
	steps   atomic.Int32
}

func (b *fakeBridge) Key() string { return b.key }

func (b *fakeBridge) Sources(context.Context) ([]relayer.Source, error) { return b.sources, nil }

func (b *fakeBridge) Step(ctx context.Context, t *relayer.Transfer) (relayer.StepResult, error) {
	b.steps.Add(1)
	return b.stepFn(ctx, t)
}

// fakeSource emits a fixed set of events and then blocks until ctx is done.
type fakeSource struct {
	chainID string
	events  []*relayer.Event
}

func (s *fakeSource) StreamEvents(_ context.Context, _ string) (<-chan *relayer.Event, <-chan error) {
	eventCh := make(chan *relayer.Event, len(s.events))
	for _, e := range s.events {
		eventCh <- e
	}
	return eventCh, make(chan error, 1)
}

func (s *fakeSource) GetChainID() string { return s.chainID }

func (*fakeSource) ExtractOffset(event *relayer.Event) string {
	return strconv.FormatUint(event.SourceBlockNumber, 10)
}

func newDriverRegistry(t *testing.T, bridges ...relayer.TokenBridge) *relayer.Registry {
	t.Helper()
	registry := relayer.NewRegistry()
	for _, b := range bridges {
		if err := registry.Register(b); err != nil {
			t.Fatalf("Register(%s) failed: %v", b.Key(), err)
		}
	}
	return registry
}

func newTestDriver(cfg *relayer.Config, registry *relayer.Registry, store BridgeStore) *Driver {
	return NewDriver(cfg, registry, store, NewNopMetrics(), zap.NewNop())
}

func steppableTransfer(bridgeKey string) *relayer.Transfer {
	return &relayer.Transfer{
		ID:          "t-1",
		BridgeKey:   bridgeKey,
		TokenSymbol: "USDCX",
		Direction:   relayer.DirectionEthereumToCanton,
		Status:      relayer.TransferStatusPending,
		CreatedAt:   time.Now().Add(-time.Minute),
	}
}

func TestDriver_StepDueTransfers_AppliesResult(t *testing.T) {
	ctx := context.Background()
	bridge := &fakeBridge{
		key: "fake",
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			return relayer.StepResult{
				Status:     relayer.TransferStatusInProgress,
				Stage:      "awaiting_attestation",
				Metadata:   map[string]string{"attestation_id": "att-1"},
				RetryAfter: 45 * time.Second,
			}, nil
		},
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetSteppableTransfers(mock.Anything, []string{"fake"}, defaultStepBatchLimit).
		Return([]*relayer.Transfer{steppableTransfer("fake")}, nil).Once()

	before := time.Now()
	store.EXPECT().ApplyStep(mock.Anything, "t-1", mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, res relayer.StepResult, nextStepAt time.Time) error {
			if res.Status != relayer.TransferStatusInProgress || res.Stage != "awaiting_attestation" {
				t.Errorf("unexpected step result: %+v", res)
			}
			if res.Metadata["attestation_id"] != "att-1" {
				t.Errorf("metadata not propagated: %+v", res.Metadata)
			}
			if nextStepAt.Before(before.Add(45*time.Second)) || nextStepAt.After(time.Now().Add(46*time.Second)) {
				t.Errorf("nextStepAt = %v, want ~now+45s", nextStepAt)
			}
			return nil
		}).Once()

	d := newTestDriver(&relayer.Config{}, newDriverRegistry(t, bridge), store)
	d.stepDueTransfers(ctx)

	if bridge.steps.Load() != 1 {
		t.Fatalf("Step called %d times, want 1", bridge.steps.Load())
	}
}

func TestDriver_StepDueTransfers_TerminalCompletion(t *testing.T) {
	ctx := context.Background()
	destTx := "0xdest"
	bridge := &fakeBridge{
		key: "fake",
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			return relayer.StepResult{
				Status:     relayer.TransferStatusCompleted,
				Stage:      "minted",
				DestTxHash: &destTx,
			}, nil
		},
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetSteppableTransfers(mock.Anything, []string{"fake"}, defaultStepBatchLimit).
		Return([]*relayer.Transfer{steppableTransfer("fake")}, nil).Once()
	store.EXPECT().ApplyStep(mock.Anything, "t-1", mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, res relayer.StepResult, _ time.Time) error {
			if res.Status != relayer.TransferStatusCompleted || res.DestTxHash == nil || *res.DestTxHash != destTx {
				t.Errorf("unexpected terminal result: %+v", res)
			}
			return nil
		}).Once()

	d := newTestDriver(&relayer.Config{}, newDriverRegistry(t, bridge), store)
	d.stepDueTransfers(ctx)
}

func TestDriver_StepError_RecordsRetry(t *testing.T) {
	ctx := context.Background()
	bridge := &fakeBridge{
		key: "fake",
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			return relayer.StepResult{}, errors.New("attestation api down")
		},
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetSteppableTransfers(mock.Anything, []string{"fake"}, defaultStepBatchLimit).
		Return([]*relayer.Transfer{steppableTransfer("fake")}, nil).Once()
	store.EXPECT().RecordStepError(mock.Anything, "t-1", mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, errMsg string, _ time.Time) error {
			if !strings.Contains(errMsg, "attestation api down") {
				t.Errorf("errMsg = %q, want the step error", errMsg)
			}
			return nil
		}).Once()

	d := newTestDriver(&relayer.Config{RetryDelay: time.Minute}, newDriverRegistry(t, bridge), store)
	d.stepDueTransfers(ctx)
}

func TestDriver_EmptyStatus_TreatedAsAdapterError(t *testing.T) {
	ctx := context.Background()
	bridge := &fakeBridge{
		key: "fake",
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			return relayer.StepResult{Stage: "oops"}, nil
		},
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetSteppableTransfers(mock.Anything, []string{"fake"}, defaultStepBatchLimit).
		Return([]*relayer.Transfer{steppableTransfer("fake")}, nil).Once()
	store.EXPECT().RecordStepError(mock.Anything, "t-1", mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, errMsg string, _ time.Time) error {
			if !strings.Contains(errMsg, "empty status") {
				t.Errorf("errMsg = %q, want empty-status error", errMsg)
			}
			return nil
		}).Once()

	d := newTestDriver(&relayer.Config{}, newDriverRegistry(t, bridge), store)
	d.stepDueTransfers(ctx)
}

func TestDriver_MaxRetriesExceeded_MarksFailed(t *testing.T) {
	ctx := context.Background()
	bridge := &fakeBridge{
		key: "fake",
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			t.Error("Step should not be called once retries are exhausted")
			return relayer.StepResult{}, nil
		},
	}

	exhausted := steppableTransfer("fake")
	exhausted.RetryCount = 3

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetSteppableTransfers(mock.Anything, []string{"fake"}, defaultStepBatchLimit).
		Return([]*relayer.Transfer{exhausted}, nil).Once()
	store.EXPECT().UpdateTransferStatus(mock.Anything, "t-1", relayer.TransferStatusFailed, mock.Anything, mock.Anything).
		Return(nil).Once()

	d := newTestDriver(&relayer.Config{MaxRetries: 3}, newDriverRegistry(t, bridge), store)
	d.stepDueTransfers(ctx)

	if bridge.steps.Load() != 0 {
		t.Fatalf("Step called %d times, want 0", bridge.steps.Load())
	}
}

func TestDriver_OrphanedBridgeKey_MarksFailed(t *testing.T) {
	ctx := context.Background()
	bridge := &fakeBridge{
		key: "fake",
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			return relayer.StepResult{Status: relayer.TransferStatusCompleted}, nil
		},
	}

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetSteppableTransfers(mock.Anything, []string{"fake"}, defaultStepBatchLimit).
		Return([]*relayer.Transfer{steppableTransfer("other")}, nil).Once()
	store.EXPECT().UpdateTransferStatus(mock.Anything, "t-1", relayer.TransferStatusFailed, mock.Anything, mock.Anything).
		Return(nil).Once()

	d := newTestDriver(&relayer.Config{}, newDriverRegistry(t, bridge), store)
	d.stepDueTransfers(ctx)
}

func TestDriver_Ingest_CreatesTransferAndPersistsOffset(t *testing.T) {
	ctx := context.Background()

	event := &relayer.Event{
		ID:                "0xdeposit-0",
		TokenSymbol:       "USDCX",
		Direction:         relayer.DirectionEthereumToCanton,
		SourceChain:       relayer.ChainEthereum,
		DestinationChain:  relayer.ChainCanton,
		SourceTxHash:      "0xdeposit",
		Amount:            "1000000",
		SourceBlockNumber: 42,
	}
	bridge := &fakeBridge{
		key:     "fake",
		sources: []relayer.Source{&fakeSource{chainID: relayer.ChainEthereum, events: []*relayer.Event{event}}},
		stepFn: func(_ context.Context, _ *relayer.Transfer) (relayer.StepResult, error) {
			return relayer.StepResult{Status: relayer.TransferStatusCompleted}, nil
		},
	}

	offsetSaved := make(chan struct{})

	store := relayermocks.NewBridgeStore(t)
	store.EXPECT().GetChainState(mock.Anything, "fake:ethereum").Return(nil, nil).Once()
	store.EXPECT().CreateTransfer(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, tr *relayer.Transfer) (bool, error) {
			if tr.BridgeKey != "fake" || tr.TokenSymbol != "USDCX" || tr.ID != event.ID {
				t.Errorf("ingested transfer mismatch: %+v", tr)
			}
			return true, nil
		}).Once()
	store.EXPECT().SetChainState(mock.Anything, "fake:ethereum", uint64(42), "42").
		RunAndReturn(func(_ context.Context, _ string, _ uint64, _ string) error {
			close(offsetSaved)
			return nil
		}).Once()

	d := newTestDriver(&relayer.Config{}, newDriverRegistry(t, bridge), store)
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer d.Stop()

	select {
	case <-offsetSaved:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for ingest to persist the offset")
	}
}
