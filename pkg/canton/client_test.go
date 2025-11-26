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

func TestClient_SubmitWithdrawal(t *testing.T) {
	mockCmdService := &MockCommandService{
		SubmitAndWaitFunc: func(ctx context.Context, in *lapi.SubmitAndWaitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
			// Verify request fields
			if len(in.Commands.Commands) != 1 {
				t.Errorf("Expected 1 command, got %d", len(in.Commands.Commands))
			}
			cmd := in.Commands.Commands[0].GetExercise()
			if cmd == nil {
				t.Errorf("Expected Exercise command")
			}
			if cmd.Choice != "ConfirmWithdrawal" {
				t.Errorf("Expected choice ConfirmWithdrawal, got %s", cmd.Choice)
			}
			return &emptypb.Empty{}, nil
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
		logger:         zap.NewNop(),
		commandService: mockCmdService,
	}

	req := &WithdrawalRequest{
		EthTxHash:   "0x123",
		EthSender:   "0xabc",
		Recipient:   "Bob",
		Amount:      "100",
		Nonce:       1,
		EthChainID:  1,
		TokenSymbol: "ETH",
	}

	err := client.SubmitWithdrawal(context.Background(), req)
	if err != nil {
		t.Errorf("SubmitWithdrawal failed: %v", err)
	}
}

func TestClient_StreamDeposits(t *testing.T) {
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

	depositCh, errCh := client.StreamDeposits(ctx, "BEGIN")

	// Wait for completion
	select {
	case <-depositCh:
		// Channel closed
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamDeposits returned error: %v", err)
		}
	case <-ctx.Done():
		t.Errorf("Test timed out")
	}
}

func TestClient_StreamDeposits_WithData(t *testing.T) {
	// Create a fake deposit event
	depositRecord := &lapi.Record{
		Fields: []*lapi.RecordField{
			{Label: "ethRecipient", Value: TextValue("0xRecipient")},
			{Label: "tokenSymbol", Value: TextValue("ETH")},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "nonce", Value: Int64Value(42)},
		},
	}

	event := &lapi.Event{
		Event: &lapi.Event_Created{
			Created: &lapi.CreatedEvent{
				EventId: "event-1",
				TemplateId: &lapi.Identifier{
					EntityName: "DepositRequest",
				},
				CreateArguments: depositRecord,
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

	depositCh, errCh := client.StreamDeposits(ctx, "BEGIN")

	select {
	case deposit := <-depositCh:
		if deposit.EventID != "event-1" {
			t.Errorf("Expected EventID event-1, got %s", deposit.EventID)
		}
		if deposit.Amount != "50.00" {
			t.Errorf("Expected Amount 50.00, got %s", deposit.Amount)
		}
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamDeposits returned error: %v", err)
		}
	case <-ctx.Done():
		t.Errorf("Test timed out waiting for deposit")
	}
}
