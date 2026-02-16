package relayer

import (
	"context"
	canton "github.com/chainsafe/canton-middleware/pkg/canton-sdk/bridge"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"testing"
)

func TestCantonDestination_SubmitTransfer(t *testing.T) {
	// Setup mock client with issuer-centric flow methods
	mockClient := &MockCantonClient{
		CreatePendingDepositFunc: func(ctx context.Context, req canton.CreatePendingDepositRequest) (*canton.PendingDeposit, error) {
			if req.Fingerprint != "BobFingerprint" {
				t.Errorf("Expected Fingerprint BobFingerprint, got %s", req.Fingerprint)
			}
			if req.EvmTxHash != "0xsrc-tx-hash" {
				t.Errorf("Expected EvmTxHash 0xsrc-tx-hash, got %s", req.EvmTxHash)
			}
			return &canton.PendingDeposit{
				ContractID: "deposit-cid-123",
			}, nil
		},
		ProcessDepositAndMintFunc: func(ctx context.Context, req canton.ProcessDepositRequest) (*canton.ProcessedDeposit, error) {
			if req.DepositCID != "deposit-cid-123" {
				t.Errorf("Expected DepositCid deposit-cid-123, got %s", req.DepositCID)
			}
			if req.MappingCID != "mapping-cid-123" {
				t.Errorf("Expected MappingCid mapping-cid-123, got %s", req.MappingCID)
			}
			return &canton.ProcessedDeposit{ContractID: "holding-cid-123"}, nil
		},
	}

	cfg := &config.EthereumConfig{}
	dest := NewCantonDestination(mockClient, cfg, "RelayerParty", "canton")

	event := &Event{
		ID:           "event-1",
		SourceChain:  "ethereum",
		SourceTxHash: "0xsrc-tx-hash",
		Sender:       "0xAlice",
		Recipient:    "BobFingerprint",        // This is now the fingerprint from EVM event
		Amount:       "100000000000000000000", // 100 tokens
		TokenAddress: "0xToken",
	}

	holdingCid, err := dest.SubmitTransfer(context.Background(), event)
	if err != nil {
		t.Errorf("SubmitTransfer failed: %v", err)
	}
	if holdingCid != "holding-cid-123" {
		t.Errorf("Expected holdingCid holding-cid-123, got %s", holdingCid)
	}
}

func TestCantonSource_StreamEvents_Error(t *testing.T) {
	// Setup mock that returns error
	withdrawalCh := make(chan *canton.WithdrawalEvent)

	// Simulate error
	go func() {
		close(withdrawalCh)
	}()

	mockClient := &MockCantonClient{
		StreamWithdrawalEventsFunc: func(ctx context.Context, offset string) <-chan *canton.WithdrawalEvent {
			return withdrawalCh
		},
	}

	source := NewCantonSource(mockClient, "0xTokenAddress", "canton")
	eventCh, errCh := source.StreamEvents(context.Background(), "BEGIN")

	// Expect no events and no errors, just the channel to close
	for {
		select {
		case _, ok := <-eventCh:
			if ok {
				t.Error("Expected no events")
			} else {
				// Channel closed, test passed
				return
			}
		case err, ok := <-errCh:
			if ok {
				t.Errorf("Expected no error, got %v", err)
			}
		}
	}
}
