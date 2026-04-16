//go:build e2e

package dsl

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

const (
	pollInterval           = 500 * time.Millisecond
	relayerReadyTimeout    = 60 * time.Second
	cantonBalanceTimeout   = 60 * time.Second
	relayerTransferTimeout = 120 * time.Second
	indexerEventTimeout    = 60 * time.Second
	ethBalanceTimeout      = 120 * time.Second
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

// WaitForPartyEvent polls until the indexer has at least one event of
// eventType for partyID, then returns it. Use WaitForPartyEventMatching when
// a specific event must be selected (e.g. by ExternalTxID or Fingerprint).
func (d *DSL) WaitForPartyEvent(ctx context.Context, t *testing.T, partyID string, eventType indexer.EventType) *indexer.ParsedEvent {
	t.Helper()
	return d.WaitForPartyEventMatching(ctx, t, partyID, eventType, func(_ *indexer.ParsedEvent) bool { return true })
}

// WaitForPartyEventMatching polls until an event of eventType for partyID
// satisfies the match predicate, then returns it. Each poll scans the first
// 50 results so a single matching event among earlier entries is found quickly.
func (d *DSL) WaitForPartyEventMatching(
	ctx context.Context,
	t *testing.T,
	partyID string,
	eventType indexer.EventType,
	match func(*indexer.ParsedEvent) bool,
) *indexer.ParsedEvent {
	t.Helper()
	if d.indexer == nil {
		t.Fatal("WaitForPartyEventMatching not available: Indexer shim not initialized (use NewFullStack)")
		return nil // unreachable; t.Fatal calls runtime.Goexit
	}
	deadline := time.Now().Add(indexerEventTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		page, err := d.indexer.ListPartyEvents(ctx, partyID, eventType, 1, 50)
		if err == nil && page != nil {
			for _, ev := range page.Items {
				if match(ev) {
					return ev
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context canceled waiting for %s event for party %s", eventType, partyID)
		case <-ticker.C:
		}
	}
	t.Fatalf("timeout waiting for %s event for party %s", eventType, partyID)
	return nil // unreachable; t.Fatalf calls runtime.Goexit
}

// WaitForHolderCount polls until GetToken reports HolderCount == expected for
// the token identified by (admin, id).
func (d *DSL) WaitForHolderCount(ctx context.Context, t *testing.T, admin, id string, expected int64) {
	t.Helper()
	if d.indexer == nil {
		t.Fatal("WaitForHolderCount not available: Indexer shim not initialized (use NewFullStack)")
		return
	}
	deadline := time.Now().Add(indexerEventTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastCount int64
	for time.Now().Before(deadline) {
		tok, err := d.indexer.GetToken(ctx, admin, id)
		if err == nil && tok != nil {
			lastCount = tok.HolderCount
			if tok.HolderCount == expected {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context canceled waiting for holder count %d (token %s/%s)", expected, admin, id)
		case <-ticker.C:
		}
	}
	t.Fatalf("timeout waiting for holder count %d: last=%d (token %s/%s)", expected, lastCount, admin, id)
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

// WaitForEthBalance polls the Anvil ERC-20 balance of ownerAddr for tokenAddr
// until it is >= minWei, or the 120s timeout is reached. minWei is expressed in
// the token's smallest unit (wei for 18-decimal tokens). Use this to confirm
// that the relayer has released tokens on Ethereum after a Canton withdrawal.
func (d *DSL) WaitForEthBalance(ctx context.Context, t *testing.T, tokenAddr, ownerAddr common.Address, minWei *big.Int) {
	t.Helper()
	if d.anvil == nil {
		t.Fatal("WaitForEthBalance not available: Anvil shim not initialized (use NewFullStack)")
		return
	}
	deadline := time.Now().Add(ethBalanceTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastBal *big.Int
	for time.Now().Before(deadline) {
		bal, err := d.anvil.ERC20Balance(ctx, tokenAddr, ownerAddr)
		if err == nil && bal != nil {
			lastBal = bal
			if bal.Cmp(minWei) >= 0 {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("context canceled waiting for Eth ERC-20 balance")
		case <-ticker.C:
		}
	}
	t.Fatalf("WaitForEthBalance: timed out waiting for balance >= %s, last=%v (token=%s owner=%s)",
		minWei, lastBal, tokenAddr.Hex(), ownerAddr.Hex())
}

// SignTransactionHash signs a hex-encoded transaction hash with the ECDSA
// private key. Used by tests to produce the Canton signature for ExecuteTransfer.
func SignTransactionHash(hexKey, txHashHex string) (string, error) {
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return "", fmt.Errorf("parse key: %w", err)
	}
	hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHashHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("decode tx hash: %w", err)
	}
	sig, err := crypto.Sign(hashBytes, key)
	if err != nil {
		return "", fmt.Errorf("sign tx hash: %w", err)
	}
	return "0x" + hex.EncodeToString(sig), nil
}
