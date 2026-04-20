package engine_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/relayer/engine"
	relayermocks "github.com/chainsafe/canton-middleware/pkg/relayer/engine/mocks"
)

func TestProcessor_Start_ProcessesEventSuccess(t *testing.T) {
	ctx := context.Background()
	source := relayermocks.NewSource(t)
	destination := relayermocks.NewDestination(t)
	store := relayermocks.NewBridgeStore(t)

	eventCh := make(chan *relayer.Event, 1)
	errCh := make(chan error)

	eventCh <- &relayer.Event{
		ID:                "event-1",
		SourceTxHash:      "0xsource",
		TokenAddress:      "0xtoken",
		Amount:            "100",
		Sender:            "alice",
		Recipient:         "bob",
		Nonce:             9,
		SourceBlockNumber: 101,
	}
	close(eventCh)

	source.EXPECT().GetChainID().Return(relayer.ChainCanton).Maybe()
	destination.EXPECT().GetChainID().Return(relayer.ChainEthereum).Maybe()
	source.EXPECT().StreamEvents(ctx, "0").Return((<-chan *relayer.Event)(eventCh), (<-chan error)(errCh)).Once()
	store.EXPECT().CreateTransfer(ctx, mock.AnythingOfType("*relayer.Transfer")).Return(true, nil).Once()
	destination.EXPECT().SubmitTransfer(ctx, mock.AnythingOfType("*relayer.Event")).Return("0xdest", false, nil).Once()
	store.EXPECT().UpdateTransferStatus(
		ctx,
		"event-1",
		relayer.TransferStatusCompleted,
		mock.MatchedBy(func(v *string) bool { return v != nil && *v == "0xdest" }),
		(*string)(nil),
	).Return(nil).Once()
	source.EXPECT().ExtractOffset(mock.AnythingOfType("*relayer.Event")).Return("101").Once()

	var persistedChain string
	var persistedOffset string
	var hookCalled bool

	processor := engine.NewProcessor(source, destination, store, engine.NewNopMetrics(), zap.NewNop(), "processor_test", relayer.DirectionCantonToEthereum).
		WithOffsetUpdate(func(_ context.Context, chainID string, offset string) error {
			persistedChain = chainID
			persistedOffset = offset
			return nil
		}).
		WithPostSubmit(func(_ context.Context, event *relayer.Event, destTxHash string) error {
			if event.ID != "event-1" || destTxHash != "0xdest" {
				t.Fatalf("unexpected post-submit arguments: event=%+v tx=%s", event, destTxHash)
			}
			hookCalled = true
			return nil
		})

	if err := processor.Start(ctx, "0"); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if !hookCalled {
		t.Fatalf("expected post-submit hook to be called")
	}
	if persistedChain != relayer.ChainCanton || persistedOffset != "101" {
		t.Fatalf("unexpected persisted offset: chain=%s offset=%s", persistedChain, persistedOffset)
	}
}

func TestProcessor_Start_DuplicateTransferPersistsOffsetOnce(t *testing.T) {
	ctx := context.Background()
	source := relayermocks.NewSource(t)
	destination := relayermocks.NewDestination(t)
	store := relayermocks.NewBridgeStore(t)

	eventCh := make(chan *relayer.Event, 2)
	errCh := make(chan error)

	eventCh <- &relayer.Event{ID: "event-2", SourceTxHash: "0xsource"}
	eventCh <- &relayer.Event{ID: "event-3", SourceTxHash: "0xsource"}
	close(eventCh)

	source.EXPECT().GetChainID().Return(relayer.ChainCanton).Maybe()
	destination.EXPECT().GetChainID().Return(relayer.ChainEthereum).Maybe()
	source.EXPECT().StreamEvents(ctx, "0").Return((<-chan *relayer.Event)(eventCh), (<-chan error)(errCh)).Once()
	store.EXPECT().CreateTransfer(ctx, mock.AnythingOfType("*relayer.Transfer")).Return(false, nil).Twice()
	source.EXPECT().ExtractOffset(mock.AnythingOfType("*relayer.Event")).Return("202").Twice()

	persistCalls := 0
	processor := engine.NewProcessor(source, destination, store, engine.NewNopMetrics(), zap.NewNop(), "processor_test", relayer.DirectionCantonToEthereum).
		WithOffsetUpdate(func(_ context.Context, _ string, _ string) error {
			persistCalls++
			return nil
		})

	if err := processor.Start(ctx, "0"); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	if persistCalls != 1 {
		t.Fatalf("expected exactly one offset persist call, got %d", persistCalls)
	}
}

func TestProcessor_Start_SubmitErrorKeepsTransferPending(t *testing.T) {
	ctx := context.Background()
	source := relayermocks.NewSource(t)
	destination := relayermocks.NewDestination(t)
	store := relayermocks.NewBridgeStore(t)

	eventCh := make(chan *relayer.Event, 1)
	errCh := make(chan error)
	eventCh <- &relayer.Event{ID: "event-4", SourceTxHash: "0xsource"}
	close(eventCh)

	source.EXPECT().GetChainID().Return(relayer.ChainCanton).Maybe()
	destination.EXPECT().GetChainID().Return(relayer.ChainEthereum).Maybe()
	source.EXPECT().StreamEvents(ctx, "0").Return((<-chan *relayer.Event)(eventCh), (<-chan error)(errCh)).Once()
	store.EXPECT().CreateTransfer(ctx, mock.AnythingOfType("*relayer.Transfer")).Return(true, nil).Once()
	destination.EXPECT().SubmitTransfer(ctx, mock.AnythingOfType("*relayer.Event")).Return("", false, errors.New("submit failed")).Once()
	store.EXPECT().UpdateTransferStatus(
		ctx,
		"event-4",
		relayer.TransferStatusPending,
		(*string)(nil),
		mock.MatchedBy(func(v *string) bool { return v != nil && strings.Contains(*v, "submit failed") }),
	).Return(nil).Once()

	processor := engine.NewProcessor(source, destination, store, engine.NewNopMetrics(), zap.NewNop(), "processor_test", relayer.DirectionCantonToEthereum)
	if err := processor.Start(ctx, "0"); err != nil {
		t.Fatalf("Start() should continue after event processing error, got %v", err)
	}
}

func TestProcessor_Start_ReturnsSourceStreamError(t *testing.T) {
	ctx := context.Background()
	source := relayermocks.NewSource(t)
	destination := relayermocks.NewDestination(t)
	store := relayermocks.NewBridgeStore(t)

	eventCh := make(chan *relayer.Event)
	errCh := make(chan error, 1)
	errCh <- errors.New("stream exploded")

	source.EXPECT().GetChainID().Return(relayer.ChainCanton).Maybe()
	destination.EXPECT().GetChainID().Return(relayer.ChainEthereum).Maybe()
	source.EXPECT().StreamEvents(ctx, "10").Return((<-chan *relayer.Event)(eventCh), (<-chan error)(errCh)).Once()

	processor := engine.NewProcessor(source, destination, store, engine.NewNopMetrics(), zap.NewNop(), "processor_test", relayer.DirectionCantonToEthereum)
	err := processor.Start(ctx, "10")
	if err == nil || !strings.Contains(err.Error(), "source stream error") {
		t.Fatalf("expected source stream error, got %v", err)
	}
}
