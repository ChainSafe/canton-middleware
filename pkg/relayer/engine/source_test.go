package engine_test

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"

	bridgesdk "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/relayer/engine"
	relayermocks "github.com/chainsafe/canton-middleware/pkg/relayer/engine/mocks"
)

func TestCantonSource_ExtractOffset(t *testing.T) {
	source := engine.NewCantonSource(nil, "0xtoken", relayer.ChainCanton)
	if got := source.ExtractOffset(&relayer.Event{ID: "12345-0"}); got != "12345" {
		t.Fatalf("expected offset 12345, got %q", got)
	}
	if got := source.ExtractOffset(&relayer.Event{ID: "bad"}); got != "" {
		t.Fatalf("expected empty offset for malformed event id, got %q", got)
	}
}

func TestCantonSource_StreamEvents_MapsWithdrawalAndSignalsUnexpectedClose(t *testing.T) {
	ctx := context.Background()
	bridgeClient := relayermocks.NewCantonBridge(t)
	withdrawalCh := make(chan *bridgesdk.WithdrawalEvent, 1)

	bridgeClient.EXPECT().StreamWithdrawalEvents(ctx, "50").Return((<-chan *bridgesdk.WithdrawalEvent)(withdrawalCh))

	source := engine.NewCantonSource(bridgeClient, "0xtoken", relayer.ChainCanton)
	eventCh, errCh := source.StreamEvents(ctx, "50")

	withdrawalCh <- &bridgesdk.WithdrawalEvent{
		ContractID:     "cid-1",
		EventID:        "51-0",
		TransactionID:  "tx-1",
		UserParty:      "user-party",
		EvmDestination: "0xrecipient",
		Amount:         "100",
	}
	close(withdrawalCh)

	select {
	case got := <-eventCh:
		if got == nil {
			t.Fatalf("expected event, got nil")
		}
		if got.ID != "51-0" || got.TransactionID != "tx-1" || got.SourceTxHash != "cid-1" || got.SourceContractID != "cid-1" {
			t.Fatalf("unexpected mapped event: %+v", got)
		}
		if got.TokenAddress != "0xtoken" || got.Sender != "user-party" || got.Recipient != "0xrecipient" || got.Amount != "100" {
			t.Fatalf("unexpected mapped event fields: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mapped canton event")
	}

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "closed unexpectedly") {
			t.Fatalf("unexpected stream close error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canton stream close error")
	}
}

func TestCantonSource_StreamEvents_ContextCancellationDoesNotReportCloseError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bridgeClient := relayermocks.NewCantonBridge(t)
	withdrawalCh := make(chan *bridgesdk.WithdrawalEvent)

	bridgeClient.EXPECT().StreamWithdrawalEvents(ctx, "").Return((<-chan *bridgesdk.WithdrawalEvent)(withdrawalCh))

	source := engine.NewCantonSource(bridgeClient, "0xtoken", relayer.ChainCanton)
	_, errCh := source.StreamEvents(ctx, "")

	cancel()

	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			t.Fatalf("expected no error on context cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for canton stream shutdown")
	}
}

func TestEthereumSource_ExtractOffset(t *testing.T) {
	src := engine.NewEthereumSource(nil, &config.EthereumConfig{}, relayer.ChainEthereum)
	if got := src.ExtractOffset(&relayer.Event{SourceBlockNumber: 0}); got != "" {
		t.Fatalf("expected empty offset for block 0, got %q", got)
	}
	if got := src.ExtractOffset(&relayer.Event{SourceBlockNumber: 123}); got != "123" {
		t.Fatalf("expected offset 123, got %q", got)
	}
}

func TestEthereumSource_StreamEvents_InvalidOffset(t *testing.T) {
	ctx := context.Background()
	ethClient := relayermocks.NewEthereumBridgeClient(t)
	source := engine.NewEthereumSource(ethClient, &config.EthereumConfig{}, relayer.ChainEthereum)

	_, errCh := source.StreamEvents(ctx, "not-a-number")

	select {
	case err := <-errCh:
		if err == nil || err.Error() == "" {
			t.Fatalf("expected parse error for invalid offset")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for invalid offset error")
	}
}

func TestEthereumSource_StreamEvents_MapsDepositEvent(t *testing.T) {
	ctx := context.Background()
	ethClient := relayermocks.NewEthereumBridgeClient(t)

	deposit := &ethereum.DepositEvent{
		Token:       common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Sender:      common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Amount:      big.NewInt(1000),
		Nonce:       big.NewInt(7),
		BlockNumber: 42,
		TxHash:      common.HexToHash("0xabc"),
		LogIndex:    3,
	}
	deposit.CantonRecipient[0] = 0xaa
	deposit.CantonRecipient[1] = 0xbb

	ethClient.EXPECT().
		WatchDepositEvents(ctx, uint64(12), mock.Anything).
		RunAndReturn(func(_ context.Context, _ uint64, handler func(*ethereum.DepositEvent) error) error {
			return handler(deposit)
		})

	source := engine.NewEthereumSource(ethClient, &config.EthereumConfig{}, relayer.ChainEthereum)
	eventCh, errCh := source.StreamEvents(ctx, "12")

	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			t.Fatalf("unexpected error from ethereum stream: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ethereum stream to close")
	}

	select {
	case event, ok := <-eventCh:
		if !ok {
			t.Fatal("expected mapped event, channel closed")
		}
		if event.ID != deposit.TxHash.Hex()+"-3" {
			t.Fatalf("unexpected event ID: got %s", event.ID)
		}
		if event.SourceTxHash != deposit.TxHash.Hex() || event.TokenAddress != deposit.Token.Hex() || event.Amount != "1000" {
			t.Fatalf("unexpected mapped event values: %+v", event)
		}
		if event.Sender != deposit.Sender.Hex() || event.Recipient == "" || event.Nonce != 7 || event.SourceBlockNumber != 42 {
			t.Fatalf("unexpected sender/recipient/nonce/block in mapped event: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mapped ethereum event")
	}
}
