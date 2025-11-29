package canton

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
	lapiv1 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestClient_SubmitMintProposal(t *testing.T) {
	mockCmdService := &MockCommandService{
		SubmitAndWaitFunc: func(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
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
			return &emptypb.Empty{}, nil
		},
	}

	mockACSClient := &MockGetActiveContractsClient{
		RecvFunc: func() (*lapi.GetActiveContractsResponse, error) {
			return &lapi.GetActiveContractsResponse{
				ContractEntry: &lapi.GetActiveContractsResponse_ActiveContract{
					ActiveContract: &lapi.ActiveContract{
						CreatedEvent: &lapiv1.CreatedEvent{
							ContractId: "config-cid",
						},
					},
				},
			}, nil
		},
	}

	mockStateService := &MockStateService{
		GetActiveContractsFunc: func(ctx context.Context, in *lapi.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetActiveContractsResponse], error) {
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
		RecvFunc: func() (*lapi.GetUpdatesResponse, error) {
			return nil, io.EOF // End of stream immediately for this test
		},
	}

	// Setup mock update service
	mockUpdateService := &MockUpdateService{
		GetUpdatesFunc: func(ctx context.Context, in *lapi.GetUpdatesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetUpdatesResponse], error) {
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
	burnRecord := &lapiv1.Record{
		Fields: []*lapiv1.RecordField{
			{Label: "operator", Value: PartyValue("Alice")},
			{Label: "owner", Value: PartyValue("Bob")},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "destination", Value: TextValue("0xRecipient")},
			{Label: "reference", Value: TextValue("ref-123")},
		},
	}

	event := &lapiv1.Event{
		Event: &lapiv1.Event_Created{
			Created: &lapiv1.CreatedEvent{
				EventId: "event-1",
				TemplateId: &lapiv1.Identifier{
					EntityName: "BurnEvent",
				},
				CreateArguments: burnRecord,
			},
		},
	}

	tx := &lapi.Transaction{
		UpdateId: "tx-1",
		Events:   []*lapiv1.Event{event},
	}

	// Setup mock stream
	sent := false
	mockStream := &MockGetUpdatesClient{
		RecvFunc: func() (*lapi.GetUpdatesResponse, error) {
			if !sent {
				sent = true
				return &lapi.GetUpdatesResponse{
					Update: &lapi.GetUpdatesResponse_Transaction{
						Transaction: tx,
					},
				}, nil
			}
			return nil, io.EOF
		},
	}

	mockUpdateService := &MockUpdateService{
		GetUpdatesFunc: func(ctx context.Context, in *lapi.GetUpdatesRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetUpdatesResponse], error) {
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
