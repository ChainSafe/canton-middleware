package canton

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
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
				ActiveContracts: []*lapi.CreatedEvent{
					{ContractId: "config-cid"},
				},
			}, nil
		},
	}

	mockActiveContractsService := &MockActiveContractsService{
		GetActiveContractsFunc: func(ctx context.Context, in *lapi.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetActiveContractsResponse], error) {
			return mockACSClient, nil
		},
	}

	// Setup config
	cfg := &config.CantonConfig{
		RelayerParty:    "RelayerParty",
		BridgePackageID: "pkg-id",
		BridgeModule:    "Wayfinder.Bridge",
	}
	logger := zap.NewNop()

	client := &Client{
		config:                 cfg,
		logger:                 logger,
		commandService:         mockCmdService,
		activeContractsService: mockActiveContractsService,
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

// Manual Mocks

type MockActiveContractsService struct {
	lapi.ActiveContractsServiceClient
	GetActiveContractsFunc func(ctx context.Context, in *lapi.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetActiveContractsResponse], error)
}

func (m *MockActiveContractsService) GetActiveContracts(ctx context.Context, in *lapi.GetActiveContractsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetActiveContractsResponse], error) {
	if m.GetActiveContractsFunc != nil {
		return m.GetActiveContractsFunc(ctx, in, opts...)
	}
	return nil, nil
}

type MockGetActiveContractsClient struct {
	grpc.ServerStreamingClient[lapi.GetActiveContractsResponse]
	RecvFunc func() (*lapi.GetActiveContractsResponse, error)
}

func (m *MockGetActiveContractsClient) Recv() (*lapi.GetActiveContractsResponse, error) {
	if m.RecvFunc != nil {
		return m.RecvFunc()
	}
	return nil, io.EOF
}

func TestClient_StreamBurnEvents(t *testing.T) {
	// Setup mock stream
	mockStream := &MockGetTransactionsClient{
		RecvFunc: func() (*lapi.GetTransactionsResponse, error) {
			return nil, io.EOF // End of stream immediately for this test
		},
	}

	// Setup mock transaction service
	mockTxService := &MockTransactionService{
		GetTransactionsFunc: func(ctx context.Context, in *lapi.GetTransactionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetTransactionsResponse], error) {
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
		},
		logger:             zap.NewNop(),
		transactionService: mockTxService,
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
	burnRecord := &lapi.Record{
		Fields: []*lapi.RecordField{
			{Label: "operator", Value: PartyValue("Alice")},
			{Label: "owner", Value: PartyValue("Bob")},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "destination", Value: TextValue("0xRecipient")},
			{Label: "reference", Value: TextValue("ref-123")},
		},
	}

	event := &lapi.Event{
		Event: &lapi.Event_Created{
			Created: &lapi.CreatedEvent{
				EventId: "event-1",
				TemplateId: &lapi.Identifier{
					EntityName: "BurnEvent",
				},
				CreateArguments: burnRecord,
			},
		},
	}

	tx := &lapi.Transaction{
		TransactionId: "tx-1",
		Events:        []*lapi.Event{event},
	}

	// Setup mock stream
	sent := false
	mockStream := &MockGetTransactionsClient{
		RecvFunc: func() (*lapi.GetTransactionsResponse, error) {
			if !sent {
				sent = true
				return &lapi.GetTransactionsResponse{
					Transactions: []*lapi.Transaction{tx},
				}, nil
			}
			return nil, io.EOF
		},
	}

	mockTxService := &MockTransactionService{
		GetTransactionsFunc: func(ctx context.Context, in *lapi.GetTransactionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[lapi.GetTransactionsResponse], error) {
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
		},
		logger:             zap.NewNop(),
		transactionService: mockTxService,
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
