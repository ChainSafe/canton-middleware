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

func TestClient_SubmitMintProposal(t *testing.T) {
	mockCmdService := &MockCommandService{
		SubmitAndWaitFunc: func(ctx context.Context, in *lapiv2.SubmitAndWaitRequest, opts ...grpc.CallOption) (*lapiv2.SubmitAndWaitResponse, error) {
			if len(in.Commands.Commands) != 1 {
				t.Errorf("Expected 1 command, got %d", len(in.Commands.Commands))
			}
			cmd := in.Commands.Commands[0].GetExercise()
			if cmd == nil {
				t.Errorf("Expected Exercise command")
				return nil, nil
			}
			if cmd.ContractId != "config-cid" {
				t.Errorf("Expected ContractId config-cid, got %s", cmd.ContractId)
			}
			if cmd.Choice != "CreateMintProposal" {
				t.Errorf("Expected Choice CreateMintProposal, got %s", cmd.Choice)
			}
			return &lapiv2.SubmitAndWaitResponse{}, nil
		},
	}

	mockACSClient := &MockGetActiveContractsClient{
		RecvFunc: func() (*lapiv2.GetActiveContractsResponse, error) {
			return &lapiv2.GetActiveContractsResponse{
				ContractEntry: &lapiv2.GetActiveContractsResponse_ActiveContract{
					ActiveContract: &lapiv2.ActiveContract{
						CreatedEvent: &lapiv2.CreatedEvent{
							ContractId: "config-cid",
						},
					},
				},
			}, nil
		},
	}

	mockStateService := &MockStateService{
		GetLedgerEndFunc: func(ctx context.Context, in *lapiv2.GetLedgerEndRequest, opts ...grpc.CallOption) (*lapiv2.GetLedgerEndResponse, error) {
			return &lapiv2.GetLedgerEndResponse{Offset: 100}, nil
		},
		GetActiveContractsFunc: func(ctx context.Context, in *lapiv2.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapiv2.GetActiveContractsResponse], error) {
			return mockACSClient, nil
		},
	}

	// Setup config
	cfg := &config.CantonConfig{
		RelayerParty:    "RelayerParty",
		BridgePackageID: "pkg-id",
		BridgeModule:    "Wayfinder.Bridge",
		DomainID:        "domain-id",
	}
	logger := zap.NewNop()

	client := &Client{
		config:         cfg,
		logger:         logger,
		commandService: mockCmdService,
		stateService:   mockStateService,
	}

	req := &MintProposalRequest{
		Recipient: "Bob",
		Amount:    "100.0",
		Reference: "tx-hash",
	}

	err := client.SubmitMintProposal(context.Background(), req)
	if err != nil {
		t.Errorf("SubmitMintProposal failed: %v", err)
	}
}

func TestClient_StreamBurnEvents(t *testing.T) {
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

	burnCh, errCh := client.StreamBurnEvents(ctx, "BEGIN")

	// Wait for completion
	select {
	case <-burnCh:
		// Channel closed
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamBurnEvents returned error: %v", err)
		}
	case <-ctx.Done():
		t.Errorf("Test timed out")
	}
}

func TestClient_StreamBurnEvents_WithData(t *testing.T) {
	// Create a fake burn event
	burnRecord := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "operator", Value: PartyValue("Alice")},
			{Label: "owner", Value: PartyValue("Bob")},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "destination", Value: TextValue("0xRecipient")},
			{Label: "reference", Value: TextValue("ref-123")},
		},
	}

	event := &lapiv2.Event{
		Event: &lapiv2.Event_Created{
			Created: &lapiv2.CreatedEvent{
				TemplateId: &lapiv2.Identifier{
					EntityName: "BurnEvent",
				},
				CreateArguments: burnRecord,
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

	burnCh, errCh := client.StreamBurnEvents(ctx, "BEGIN")

	select {
	case burn := <-burnCh:
		if burn.EventID != "event-1" {
			t.Errorf("Expected EventID event-1, got %s", burn.EventID)
		}
		if burn.Amount != "50.00" {
			t.Errorf("Expected Amount 50.00, got %s", burn.Amount)
		}
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamBurnEvents returned error: %v", err)
		}
	case <-ctx.Done():
		t.Errorf("Test timed out waiting for burn event")
	}
}
