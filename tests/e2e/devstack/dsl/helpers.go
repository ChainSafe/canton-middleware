//go:build e2e

package dsl

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/indexer"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	pollInterval           = 500 * time.Millisecond
	relayerReadyTimeout    = 60 * time.Second
	cantonBalanceTimeout   = 60 * time.Second
	relayerTransferTimeout = 120 * time.Second
)

// WaitForRelayerReady polls until the relayer reports ready or the 60s timeout
// is reached.
func (d *DSL) WaitForRelayerReady(ctx context.Context, t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(relayerReadyTimeout)
	for time.Now().Before(deadline) {
		if d.Relayer.IsReady(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("context cancelled waiting for relayer ready")
		case <-time.After(pollInterval):
		}
	}
	t.Fatal("timeout waiting for relayer to be ready")
}

// WaitForCantonBalance polls the indexer until partyID holds at least
// minAmount for the token identified by (admin, id), or the 60s timeout
// is reached.
func (d *DSL) WaitForCantonBalance(ctx context.Context, t *testing.T, partyID, admin, id, minAmount string) {
	t.Helper()
	deadline := time.Now().Add(cantonBalanceTimeout)
	for time.Now().Before(deadline) {
		bal, err := d.Indexer.GetBalance(ctx, partyID, admin, id)
		if err == nil && bal != nil && bal.Amount >= minAmount {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("context cancelled waiting for Canton balance")
		case <-time.After(pollInterval):
		}
	}
	t.Fatalf("timeout waiting for Canton balance: party=%s admin=%s id=%s min=%s", partyID, admin, id, minAmount)
}

// WaitForRelayerTransfer polls until the relayer has a completed transfer
// matching sourceTxHash, or the 120s timeout is reached.
func (d *DSL) WaitForRelayerTransfer(ctx context.Context, t *testing.T, sourceTxHash string) {
	t.Helper()
	deadline := time.Now().Add(relayerTransferTimeout)
	for time.Now().Before(deadline) {
		transfers, err := d.Relayer.ListTransfers(ctx)
		if err == nil {
			for _, tr := range transfers {
				if strings.EqualFold(tr.SourceTxHash, sourceTxHash) && tr.Status == "completed" {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("context cancelled waiting for relayer transfer")
		case <-time.After(pollInterval):
		}
	}
	t.Fatalf("timeout waiting for relayer transfer: sourceTxHash=%s", sourceTxHash)
}

// WaitForIndexerEvent polls until the indexer has an event with the given
// contractID, or the 60s timeout is reached.
func (d *DSL) WaitForIndexerEvent(ctx context.Context, t *testing.T, contractID string) *indexer.ParsedEvent {
	t.Helper()
	deadline := time.Now().Add(cantonBalanceTimeout)
	for time.Now().Before(deadline) {
		ev, err := d.Indexer.GetEvent(ctx, contractID)
		if err == nil && ev != nil {
			return ev
		}
		select {
		case <-ctx.Done():
			t.Fatal("context cancelled waiting for indexer event")
		case <-time.After(pollInterval):
		}
	}
	t.Fatalf("timeout waiting for indexer event: contractID=%s", contractID)
	return nil
}

// signEIP191 produces a 0x-prefixed EIP-191 signature. Recovery ID is set to
// 27/28 to match the api-server's VerifyEIP191Signature expectation.
func signEIP191(hexKey, message string) (string, error) {
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return "", fmt.Errorf("parse key: %w", err)
	}
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256Hash([]byte(prefix + message))
	sig, err := crypto.Sign(hash.Bytes(), key)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	sig[64] += 27
	return "0x" + hex.EncodeToString(sig), nil
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
