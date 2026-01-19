package relayer

import (
	"context"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
)

func TestCantonDestination_SubmitTransfer(t *testing.T) {
	// Setup mock client with issuer-centric flow methods
	mockClient := &MockCantonClient{
		CreatePendingDepositFunc: func(ctx context.Context, req *canton.CreatePendingDepositRequest) (string, error) {
			if req.Fingerprint != "BobFingerprint" {
				t.Errorf("Expected Fingerprint BobFingerprint, got %s", req.Fingerprint)
			}
			if req.EvmTxHash != "0xsrc-tx-hash" {
				t.Errorf("Expected EvmTxHash 0xsrc-tx-hash, got %s", req.EvmTxHash)
			}
			return "deposit-cid-123", nil
		},
		GetFingerprintMappingFunc: func(ctx context.Context, fingerprint string) (*canton.FingerprintMapping, error) {
			if fingerprint != "BobFingerprint" {
				t.Errorf("Expected fingerprint BobFingerprint, got %s", fingerprint)
			}
			return &canton.FingerprintMapping{
				ContractID:  "mapping-cid-123",
				Issuer:      "Issuer",
				UserParty:   "Bob",
				Fingerprint: fingerprint,
			}, nil
		},
		ProcessDepositFunc: func(ctx context.Context, req *canton.ProcessDepositRequest) (string, error) {
			if req.DepositCid != "deposit-cid-123" {
				t.Errorf("Expected DepositCid deposit-cid-123, got %s", req.DepositCid)
			}
			if req.MappingCid != "mapping-cid-123" {
				t.Errorf("Expected MappingCid mapping-cid-123, got %s", req.MappingCid)
			}
			return "holding-cid-123", nil
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
