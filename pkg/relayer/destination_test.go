package relayer_test

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	bridgesdk "github.com/chainsafe/canton-middleware/pkg/cantonsdk/bridge"
	relayer "github.com/chainsafe/canton-middleware/pkg/relayer"
	relayermocks "github.com/chainsafe/canton-middleware/pkg/relayer/mocks"
)

func TestCantonDestination_SubmitTransfer_AlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	bridgeClient := relayermocks.NewCantonBridge(t)
	bridgeClient.EXPECT().IsDepositProcessed(ctx, "0xsource").Return(true, nil)

	destination := relayer.NewCantonDestination(bridgeClient, relayer.ChainCanton)
	txHash, skipped, err := destination.SubmitTransfer(ctx, &relayer.Event{
		SourceTxHash: "0xsource",
		Recipient:    "fingerprint-1",
		Amount:       "1000000000000000000",
	})
	if err != nil {
		t.Fatalf("SubmitTransfer() failed: %v", err)
	}
	if !skipped {
		t.Fatalf("expected skipped=true")
	}
	if txHash != "" {
		t.Fatalf("expected empty tx hash when skipped, got %s", txHash)
	}
}

func TestCantonDestination_SubmitTransfer_Success(t *testing.T) {
	ctx := context.Background()
	bridgeClient := relayermocks.NewCantonBridge(t)

	event := &relayer.Event{
		SourceTxHash: "0xsource",
		Recipient:    "fingerprint-1",
		Amount:       "1000000000000000000",
	}

	bridgeClient.EXPECT().IsDepositProcessed(ctx, event.SourceTxHash).Return(false, nil)
	bridgeClient.EXPECT().
		CreatePendingDeposit(ctx, bridgesdk.CreatePendingDepositRequest{
			Fingerprint: event.Recipient,
			Amount:      "1",
			EvmTxHash:   event.SourceTxHash,
		}).
		Return(&bridgesdk.PendingDeposit{ContractID: "deposit-cid", MappingCID: "mapping-cid"}, nil)
	bridgeClient.EXPECT().
		ProcessDepositAndMint(ctx, bridgesdk.ProcessDepositRequest{DepositCID: "deposit-cid", MappingCID: "mapping-cid"}).
		Return(&bridgesdk.ProcessedDeposit{ContractID: "mint-cid"}, nil)

	destination := relayer.NewCantonDestination(bridgeClient, relayer.ChainCanton)
	txHash, skipped, err := destination.SubmitTransfer(ctx, event)
	if err != nil {
		t.Fatalf("SubmitTransfer() failed: %v", err)
	}
	if skipped {
		t.Fatalf("expected skipped=false")
	}
	if txHash != "mint-cid" {
		t.Fatalf("unexpected tx hash: got %s want mint-cid", txHash)
	}
}

func TestCantonDestination_SubmitTransfer_CreatePendingDepositError(t *testing.T) {
	ctx := context.Background()
	bridgeClient := relayermocks.NewCantonBridge(t)

	bridgeClient.EXPECT().IsDepositProcessed(ctx, "0xsource").Return(false, nil)
	bridgeClient.EXPECT().
		CreatePendingDeposit(ctx, bridgesdk.CreatePendingDepositRequest{Fingerprint: "fp", Amount: "0", EvmTxHash: "0xsource"}).
		Return(nil, errors.New("boom"))

	destination := relayer.NewCantonDestination(bridgeClient, relayer.ChainCanton)
	_, _, err := destination.SubmitTransfer(ctx, &relayer.Event{SourceTxHash: "0xsource", Recipient: "fp", Amount: "0"})
	if err == nil || !strings.Contains(err.Error(), "create pending deposit") {
		t.Fatalf("expected wrapped create pending deposit error, got %v", err)
	}
}

func TestEthereumDestination_SubmitTransfer_InvalidAmount(t *testing.T) {
	ctx := context.Background()
	ethClient := relayermocks.NewEthereumBridgeClient(t)
	destination := relayer.NewEthereumDestination(ethClient, relayer.ChainEthereum)

	_, _, err := destination.SubmitTransfer(ctx, &relayer.Event{Amount: "not-a-decimal"})
	if err == nil || !strings.Contains(err.Error(), "parse amount") {
		t.Fatalf("expected parse amount error, got %v", err)
	}
}

func TestEthereumDestination_SubmitTransfer_InvalidSourceTxHash(t *testing.T) {
	ctx := context.Background()
	ethClient := relayermocks.NewEthereumBridgeClient(t)
	destination := relayer.NewEthereumDestination(ethClient, relayer.ChainEthereum)

	_, _, err := destination.SubmitTransfer(ctx, &relayer.Event{
		Amount:       "1",
		SourceTxHash: "zz",
		Recipient:    "0x1111111111111111111111111111111111111111",
		TokenAddress: "0x2222222222222222222222222222222222222222",
	})
	if err == nil || !strings.Contains(err.Error(), "decode source tx hash") {
		t.Fatalf("expected decode source tx hash error, got %v", err)
	}
}

func TestEthereumDestination_SubmitTransfer_AlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	ethClient := relayermocks.NewEthereumBridgeClient(t)

	sourceHash := strings.Repeat("ab", 32)
	var cantonTxHash [32]byte
	for i := range cantonTxHash {
		cantonTxHash[i] = 0xab
	}

	ethClient.EXPECT().IsWithdrawalProcessed(ctx, cantonTxHash).Return(true, nil)

	destination := relayer.NewEthereumDestination(ethClient, relayer.ChainEthereum)
	txHash, skipped, err := destination.SubmitTransfer(ctx, &relayer.Event{
		TokenAddress: "0x2222222222222222222222222222222222222222",
		Recipient:    "0x1111111111111111111111111111111111111111",
		Amount:       "1",
		Nonce:        7,
		SourceTxHash: sourceHash,
	})
	if err != nil {
		t.Fatalf("SubmitTransfer() failed: %v", err)
	}
	if !skipped {
		t.Fatalf("expected skipped=true")
	}
	if txHash != "" {
		t.Fatalf("expected empty tx hash for skipped transfer, got %s", txHash)
	}
}

func TestEthereumDestination_SubmitTransfer_Success(t *testing.T) {
	ctx := context.Background()
	ethClient := relayermocks.NewEthereumBridgeClient(t)

	sourceHash := strings.Repeat("01", 32)
	var cantonTxHash [32]byte
	for i := range cantonTxHash {
		cantonTxHash[i] = 0x01
	}

	wantAmount := new(big.Int).SetUint64(1500000000000000000)

	ethClient.EXPECT().IsWithdrawalProcessed(ctx, cantonTxHash).Return(false, nil)
	ethClient.EXPECT().
		WithdrawFromCanton(
			ctx,
			common.HexToAddress("0x2222222222222222222222222222222222222222"),
			common.HexToAddress("0x1111111111111111111111111111111111111111"),
			wantAmount,
			big.NewInt(9),
			cantonTxHash,
		).Return(common.HexToHash("0x1234"), nil)

	destination := relayer.NewEthereumDestination(ethClient, relayer.ChainEthereum)
	txHash, skipped, err := destination.SubmitTransfer(ctx, &relayer.Event{
		TokenAddress: "0x2222222222222222222222222222222222222222",
		Recipient:    "0x1111111111111111111111111111111111111111",
		Amount:       "1.5",
		Nonce:        9,
		SourceTxHash: sourceHash,
	})
	if err != nil {
		t.Fatalf("SubmitTransfer() failed: %v", err)
	}
	if skipped {
		t.Fatalf("expected skipped=false")
	}
	if txHash != common.HexToHash("0x1234").Hex() {
		t.Fatalf("unexpected tx hash: got %s want %s", txHash, common.HexToHash("0x1234").Hex())
	}
}
