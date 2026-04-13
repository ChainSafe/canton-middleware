//go:build e2e

package dsl

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const (
	pollInterval           = 500 * time.Millisecond
	relayerReadyTimeout    = 60 * time.Second
	cantonBalanceTimeout   = 60 * time.Second
	relayerTransferTimeout = 120 * time.Second
	indexerEventTimeout    = 60 * time.Second
)

// WaitForRelayerReady polls until the relayer reports ready or the 60s timeout
// is reached.
func (d *DSL) WaitForRelayerReady(ctx context.Context, t *testing.T) {
	t.Helper()
	if d.relayer == nil {
		t.Fatal("WaitForRelayerReady not available: Relayer shim not initialized (use NewFullStack)")
		return
	}
	deadline := time.Now().Add(relayerReadyTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		if d.relayer.IsReady(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("context canceled waiting for relayer ready")
		case <-ticker.C:
		}
	}
	t.Fatal("timeout waiting for relayer to be ready")
}

// WaitForCantonBalance polls the indexer until partyID holds at least
// minAmount for the token identified by (admin, id), or the 60s timeout
// is reached.
func (d *DSL) WaitForCantonBalance(ctx context.Context, t *testing.T, partyID, admin, id, minAmount string) {
	t.Helper()
	if d.indexer == nil {
		t.Fatal("WaitForCantonBalance not available: Indexer shim not initialized (use NewFullStack)")
		return
	}
	deadline := time.Now().Add(cantonBalanceTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastBalance string
	for time.Now().Before(deadline) {
		bal, err := d.indexer.GetBalance(ctx, partyID, admin, id)
		if err == nil && bal != nil {
			lastBalance = bal.Amount
			if amountGTE(bal.Amount, minAmount) {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("context canceled waiting for Canton balance")
		case <-ticker.C:
		}
	}
	t.Fatalf("timeout waiting for Canton balance: party=%s admin=%s id=%s min=%s last=%s",
		partyID, admin, id, minAmount, lastBalance)
}

// WaitForRelayerTransfer polls until the relayer has a completed transfer
// matching sourceTxHash, or the 120s timeout is reached.
func (d *DSL) WaitForRelayerTransfer(ctx context.Context, t *testing.T, sourceTxHash string) {
	t.Helper()
	if d.relayer == nil {
		t.Fatal("WaitForRelayerTransfer not available: Relayer shim not initialized (use NewFullStack)")
		return
	}
	deadline := time.Now().Add(relayerTransferTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastStatus relayer.TransferStatus
	for time.Now().Before(deadline) {
		transfers, err := d.relayer.ListTransfers(ctx)
		if err == nil {
			for _, tr := range transfers {
				if strings.EqualFold(tr.SourceTxHash, sourceTxHash) {
					lastStatus = tr.Status
					if tr.Status == relayer.TransferStatusCompleted {
						return
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("context canceled waiting for relayer transfer")
		case <-ticker.C:
		}
	}
	t.Fatalf("timeout waiting for relayer transfer: sourceTxHash=%s lastStatus=%s", sourceTxHash, lastStatus)
}

// WaitForIndexerEvent polls until the indexer has an event with the given
// contractID, or the 60s timeout is reached.
func (d *DSL) WaitForIndexerEvent(ctx context.Context, t *testing.T, contractID string) *indexer.ParsedEvent {
	t.Helper()
	if d.indexer == nil {
		t.Fatal("WaitForIndexerEvent not available: Indexer shim not initialized (use NewFullStack)")
		return nil // unreachable; t.Fatal calls runtime.Goexit
	}
	deadline := time.Now().Add(indexerEventTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastErr error
	for time.Now().Before(deadline) {
		ev, err := d.indexer.GetEvent(ctx, contractID)
		if err == nil && ev != nil {
			return ev
		}
		lastErr = err
		select {
		case <-ctx.Done():
			t.Fatalf("context canceled waiting for indexer event: contractID=%s", contractID)
		case <-ticker.C:
		}
	}
	t.Fatalf("timeout waiting for indexer event: contractID=%s lastErr=%v", contractID, lastErr)
	return nil // unreachable; t.Fatalf calls runtime.Goexit
}

// amountGTE returns true when amount >= min, comparing both as decimal numbers.
// String comparison is intentionally avoided: "20" > "100" lexicographically.
func amountGTE(amount, min string) bool {
	a, ok1 := new(big.Float).SetString(amount)
	m, ok2 := new(big.Float).SetString(min)
	if !ok1 || !ok2 {
		return false
	}
	return a.Cmp(m) >= 0
}

