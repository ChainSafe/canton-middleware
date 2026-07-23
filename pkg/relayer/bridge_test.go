// SPDX-License-Identifier: Apache-2.0

package relayer

import (
	"context"
	"testing"
)

const testBridgeKey = "xreserve"

type stubBridge struct {
	key string
}

func (b *stubBridge) Key() string                             { return b.key }
func (*stubBridge) Sources(context.Context) ([]Source, error) { return nil, nil }
func (*stubBridge) Step(context.Context, *Transfer) (StepResult, error) {
	return StepResult{}, nil
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register(&stubBridge{key: testBridgeKey}); err != nil {
		t.Fatalf("Register(xreserve) failed: %v", err)
	}
	if err := registry.Register(&stubBridge{key: "wayfinder"}); err != nil {
		t.Fatalf("Register(wayfinder) failed: %v", err)
	}

	if _, ok := registry.ByKey(testBridgeKey); !ok {
		t.Fatalf("ByKey(xreserve) not found")
	}
	if _, ok := registry.ByKey("unknown"); ok {
		t.Fatalf("ByKey(unknown) should not be found")
	}

	keys := registry.Keys()
	if len(keys) != 2 || keys[0] != "wayfinder" || keys[1] != testBridgeKey {
		t.Fatalf("Keys() = %v, want sorted [wayfinder xreserve]", keys)
	}

	bridges := registry.Bridges()
	if len(bridges) != 2 || bridges[0].Key() != "wayfinder" || bridges[1].Key() != testBridgeKey {
		t.Fatalf("Bridges() order mismatch")
	}
}

func TestRegistry_RejectsEmptyKey(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&stubBridge{key: ""}); err == nil {
		t.Fatalf("Register with empty key should fail")
	}
}

func TestRegistry_RejectsDuplicateKey(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&stubBridge{key: testBridgeKey}); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := registry.Register(&stubBridge{key: testBridgeKey}); err == nil {
		t.Fatalf("duplicate Register should fail")
	}
}

func TestTransferFromEvent_MapsFields(t *testing.T) {
	event := &Event{
		ID:                "tx-1-0",
		TokenSymbol:       "USDCX",
		Direction:         DirectionEthereumToCanton,
		SourceChain:       ChainEthereum,
		DestinationChain:  ChainCanton,
		SourceTxHash:      "0xabc",
		TokenAddress:      "0xtoken",
		Amount:            "1000000",
		Sender:            "0xsender",
		Recipient:         "party::recipient",
		Nonce:             7,
		SourceBlockNumber: 42,
	}

	transfer := TransferFromEvent("xreserve", event)

	if transfer.ID != event.ID {
		t.Fatalf("ID = %q, want %q", transfer.ID, event.ID)
	}
	if transfer.BridgeKey != "xreserve" {
		t.Fatalf("BridgeKey = %q, want xreserve", transfer.BridgeKey)
	}
	if transfer.TokenSymbol != "USDCX" {
		t.Fatalf("TokenSymbol = %q, want USDCX", transfer.TokenSymbol)
	}
	if transfer.Status != TransferStatusPending {
		t.Fatalf("Status = %q, want pending", transfer.Status)
	}
	if transfer.Stage != "" {
		t.Fatalf("Stage = %q, want empty", transfer.Stage)
	}
	if transfer.Direction != DirectionEthereumToCanton {
		t.Fatalf("Direction = %q, want %q", transfer.Direction, DirectionEthereumToCanton)
	}
	if transfer.Amount != "1000000" || transfer.Recipient != "party::recipient" {
		t.Fatalf("amount/recipient mismatch: %q %q", transfer.Amount, transfer.Recipient)
	}
	if transfer.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt should be set")
	}
}

func TestTransferStatus_IsTerminal(t *testing.T) {
	cases := []struct {
		status TransferStatus
		want   bool
	}{
		{TransferStatusPending, false},
		{TransferStatusInProgress, false},
		{TransferStatusCompleted, true},
		{TransferStatusFailed, true},
	}
	for _, tc := range cases {
		if got := tc.status.IsTerminal(); got != tc.want {
			t.Fatalf("IsTerminal(%s) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
