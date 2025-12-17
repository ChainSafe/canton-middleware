package canton

import (
	"context"
	"io"
	"testing"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func TestClient_StreamWithdrawalEvents(t *testing.T) {
	// Setup mock stream
	mockStream := &MockGetUpdatesClient{
		RecvFunc: func() (*lapiv2.GetUpdatesResponse, error) {
			return nil, io.EOF // End of stream immediately for this test
		},
	}

	// Setup mock update service
	mockUpdateService := &MockUpdateService{
		GetUpdatesFunc: func(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdatesClient, error) {
			return mockStream, nil
		},
	}

	client := &Client{
		config: &config.CantonConfig{
			RelayerParty:    "Alice",
			BridgePackageID: "pkg-id",
			BridgeModule:    "Module",
			BridgeContract:  "contract-id",
			LedgerID:        "ledger-id",
			ApplicationID:   "app-id",
			DomainID:        "domain-id",
		},
		logger:        zap.NewNop(),
		updateService: mockUpdateService,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	withdrawalCh := client.StreamWithdrawalEvents(ctx, "BEGIN")

	// Wait for completion
	select {
	case <-withdrawalCh:
		// Channel closed
	case <-ctx.Done():
		t.Errorf("Test timed out")
	}
}

func TestClient_StreamWithdrawalEvents_WithData(t *testing.T) {
	// Create a fake withdrawal event
	withdrawalRecord := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: PartyValue("Issuer")},
			{Label: "userParty", Value: PartyValue("Alice")},
			{Label: "evmDestination", Value: &lapiv2.Value{
				Sum: &lapiv2.Value_Record{
					Record: &lapiv2.Record{
						Fields: []*lapiv2.RecordField{
							{Label: "value", Value: TextValue("0xRecipient")},
						},
					},
				},
			}},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "fingerprint", Value: TextValue("fp-123")},
			{Label: "status", Value: &lapiv2.Value{
				Sum: &lapiv2.Value_Variant{
					Variant: &lapiv2.Variant{
						Constructor: "Pending",
					},
				},
			}},
		},
	}

	event := &lapiv2.Event{
		Event: &lapiv2.Event_Created{
			Created: &lapiv2.CreatedEvent{
				TemplateId: &lapiv2.Identifier{
					ModuleName: "Bridge.Contracts",
					EntityName: "WithdrawalEvent",
				},
				ContractId:      "cid-123",
				CreateArguments: withdrawalRecord,
				Offset:          100,
				NodeId:          1,
			},
		},
	}

	tx := &lapiv2.Transaction{
		UpdateId: "tx-1",
		Events:   []*lapiv2.Event{event},
	}

	// Setup mock stream
	sent := false
	mockStream := &MockGetUpdatesClient{
		RecvFunc: func() (*lapiv2.GetUpdatesResponse, error) {
			if !sent {
				sent = true
				return &lapiv2.GetUpdatesResponse{
					Update: &lapiv2.GetUpdatesResponse_Transaction{
						Transaction: tx,
					},
				}, nil
			}
			return nil, io.EOF
		},
	}

	mockUpdateService := &MockUpdateService{
		GetUpdatesFunc: func(ctx context.Context, in *lapiv2.GetUpdatesRequest, opts ...grpc.CallOption) (lapiv2.UpdateService_GetUpdatesClient, error) {
			return mockStream, nil
		},
	}

	client := &Client{
		config: &config.CantonConfig{
			RelayerParty:    "Issuer",
			CorePackageID:   "core-pkg-id",
			BridgePackageID: "pkg-id",
			BridgeModule:    "Module",
			BridgeContract:  "contract-id",
			LedgerID:        "ledger-id",
			ApplicationID:   "app-id",
			DomainID:        "domain-id",
		},
		logger:        zap.NewNop(),
		updateService: mockUpdateService,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	withdrawalCh := client.StreamWithdrawalEvents(ctx, "BEGIN")

	select {
	case withdrawal := <-withdrawalCh:
		if withdrawal == nil {
			t.Errorf("Expected withdrawal event, got nil")
			return
		}
		if withdrawal.Amount != "50.00" {
			t.Errorf("Expected Amount 50.00, got %s", withdrawal.Amount)
		}
		if withdrawal.EvmDestination != "0xRecipient" {
			t.Errorf("Expected destination 0xRecipient, got %s", withdrawal.EvmDestination)
		}
	case <-ctx.Done():
		t.Errorf("Test timed out waiting for withdrawal event")
	}
}
