package relayer

import (
	"context"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/canton"
	"github.com/chainsafe/canton-middleware/pkg/config"
)

func TestCantonDestination_SubmitTransfer(t *testing.T) {
	// Setup mock client
	mockClient := &MockCantonClient{
		SubmitMintProposalFunc: func(ctx context.Context, req *canton.MintProposalRequest) error {
			if req.Operator != "RelayerParty" {
				t.Errorf("Expected Operator RelayerParty, got %s", req.Operator)
			}
			if req.Recipient != "Bob" {
				t.Errorf("Expected Recipient Bob, got %s", req.Recipient)
			}
			if req.Amount != "100" {
				t.Errorf("Expected Amount 100, got %s", req.Amount)
			}
			if req.Reference != "0xsrc-tx-hash" {
				t.Errorf("Expected Reference 0xsrc-tx-hash, got %s", req.Reference)
			}
			return nil
		},
	}

	cfg := &config.EthereumConfig{}
	dest := NewCantonDestination(mockClient, cfg, "RelayerParty")

	event := &Event{
		ID:           "event-1",
		SourceChain:  "ethereum",
		SourceTxHash: "0xsrc-tx-hash",
		Sender:       "0xAlice",
		Recipient:    "Bob",
		Amount:       "100000000000000000000", // 100 tokens
		TokenAddress: "0xToken",
	}

	_, err := dest.SubmitTransfer(context.Background(), event)
	if err != nil {
		t.Errorf("SubmitTransfer failed: %v", err)
	}
}

func TestCantonSource_StreamEvents_Error(t *testing.T) {
	// Setup mock that returns error
	errCh := make(chan error, 1)
	burnCh := make(chan *canton.BurnEvent)

	// Simulate error
	go func() {
		errCh <- context.DeadlineExceeded
		close(errCh)
		close(burnCh)
	}()

	mockClient := &MockCantonClient{
		StreamBurnEventsFunc: func(ctx context.Context, startOffset string) (<-chan *canton.BurnEvent, <-chan error) {
			return burnCh, errCh
		},
	}

	source := NewCantonSource(mockClient)
	_, outErrCh := source.StreamEvents(context.Background(), "BEGIN")

	err := <-outErrCh
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got %v", err)
	}
}
