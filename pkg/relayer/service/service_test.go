// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
	"github.com/chainsafe/canton-middleware/pkg/relayer/service/mocks"
)

func validRegistration() *relayer.RegisterTransferRequest {
	return &relayer.RegisterTransferRequest{
		BridgeKey:    "xreserve",
		TokenSymbol:  "USDCX",
		Direction:    relayer.DirectionEthereumToCanton,
		SourceTxHash: "0xdeposit",
		TokenAddress: "0xusdc",
		Amount:       "12.5",
		Sender:       "0xsender",
		Recipient:    "party::recipient",
		Metadata:     map[string]string{"quote_id": "q-1"},
	}
}

func TestRegisterTransfer_CreatesPendingTransfer(t *testing.T) {
	store := mocks.NewStore(t)
	store.EXPECT().CreateTransfer(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, tr *relayer.Transfer) (bool, error) {
			if tr.ID != "0xdeposit" {
				t.Errorf("ID = %q, want default to source tx hash", tr.ID)
			}
			if tr.BridgeKey != "xreserve" || tr.Status != relayer.TransferStatusPending || tr.Stage != "" {
				t.Errorf("unexpected transfer: %+v", tr)
			}
			if tr.SourceChain != relayer.ChainEthereum || tr.DestinationChain != relayer.ChainCanton {
				t.Errorf("chains = %s -> %s, want ethereum -> canton", tr.SourceChain, tr.DestinationChain)
			}
			if tr.Metadata["quote_id"] != "q-1" {
				t.Errorf("metadata not carried: %+v", tr.Metadata)
			}
			return true, nil
		}).Once()

	svc := NewService(store, []string{"xreserve"})
	resp, err := svc.RegisterTransfer(context.Background(), validRegistration())
	if err != nil {
		t.Fatalf("RegisterTransfer failed: %v", err)
	}
	if !resp.Created || resp.Transfer == nil {
		t.Fatalf("resp = %+v, want created transfer", resp)
	}
}

func TestRegisterTransfer_IdempotentReplayReturnsExisting(t *testing.T) {
	existing := &relayer.Transfer{ID: "0xdeposit", Status: relayer.TransferStatusInProgress, Stage: "awaiting_mint"}

	store := mocks.NewStore(t)
	store.EXPECT().CreateTransfer(mock.Anything, mock.Anything).Return(false, nil).Once()
	store.EXPECT().GetTransfer(mock.Anything, "0xdeposit").Return(existing, nil).Once()

	svc := NewService(store, []string{"xreserve"})
	resp, err := svc.RegisterTransfer(context.Background(), validRegistration())
	if err != nil {
		t.Fatalf("RegisterTransfer failed: %v", err)
	}
	if resp.Created {
		t.Fatalf("replay should report created=false")
	}
	if resp.Transfer.Stage != "awaiting_mint" {
		t.Fatalf("replay should return the existing transfer, got %+v", resp.Transfer)
	}
}

func TestRegisterTransfer_Validation(t *testing.T) {
	svc := NewService(mocks.NewStore(t), []string{"xreserve"})

	cases := []struct {
		name    string
		mutate  func(*relayer.RegisterTransferRequest)
		wantErr string
	}{
		{"missing source tx", func(r *relayer.RegisterTransferRequest) { r.SourceTxHash = "" }, "source_tx_hash"},
		{"missing amount", func(r *relayer.RegisterTransferRequest) { r.Amount = "" }, "amount"},
		{"missing recipient", func(r *relayer.RegisterTransferRequest) { r.Recipient = "" }, "recipient"},
		{"bad direction", func(r *relayer.RegisterTransferRequest) { r.Direction = "sideways" }, "direction"},
		{"unknown bridge key", func(r *relayer.RegisterTransferRequest) { r.BridgeKey = "wayfinder" }, "unknown bridge key"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validRegistration()
			tc.mutate(req)
			_, err := svc.RegisterTransfer(context.Background(), req)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
