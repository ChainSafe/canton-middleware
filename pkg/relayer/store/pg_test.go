// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

func setupRelayerStore(t *testing.T) (context.Context, *PGStore) {
	t.Helper()
	requireDockerAccess(t)

	ctx := context.Background()
	db, cleanup := pgutil.SetupTestDB(t)
	t.Cleanup(cleanup)

	if err := mghelper.CreateSchema(ctx, db, &TransferDao{}, &ChainStateDao{}); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return ctx, NewStore(db)
}

func requireDockerAccess(t *testing.T) {
	t.Helper()

	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("HOME"), ".docker/run/docker.sock"),
	}

	for _, sock := range candidates {
		if sock == "" {
			continue
		}
		if _, err := os.Stat(sock); err != nil {
			continue
		}
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", sock)
		if err == nil {
			_ = conn.Close()
			return
		}
	}

	t.Skip("docker daemon socket is not accessible; skipping testcontainer-backed relayer store tests")
}

func TestPGStore_TransferLifecycle(t *testing.T) {
	ctx, store := setupRelayerStore(t)

	transferCompleted := &relayer.Transfer{
		ID:                "lifecycle-completed",
		Direction:         relayer.DirectionCantonToEthereum,
		Status:            relayer.TransferStatusPending,
		SourceChain:       relayer.ChainCanton,
		DestinationChain:  relayer.ChainEthereum,
		SourceTxHash:      "0xsource1",
		TokenAddress:      "0xtoken1",
		Amount:            "100",
		Sender:            "sender-1",
		Recipient:         "recipient-1",
		Nonce:             1,
		SourceBlockNumber: 10,
	}
	inserted, err := store.CreateTransfer(ctx, transferCompleted)
	if err != nil {
		t.Fatalf("CreateTransfer(completed seed) failed: %v", err)
	}
	if !inserted {
		t.Fatalf("CreateTransfer(completed seed) expected inserted=true")
	}

	inserted, err = store.CreateTransfer(ctx, transferCompleted)
	if err != nil {
		t.Fatalf("CreateTransfer(duplicate) failed: %v", err)
	}
	if inserted {
		t.Fatalf("CreateTransfer(duplicate) expected inserted=false")
	}

	missing, err := store.GetTransfer(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetTransfer(missing) failed: %v", err)
	}
	if missing != nil {
		t.Fatalf("GetTransfer(missing) expected nil, got %+v", missing)
	}

	destTxHash := "0xdest1"
	if err = store.UpdateTransferStatus(
		ctx,
		transferCompleted.ID,
		relayer.TransferStatusCompleted,
		&destTxHash,
		nil,
	); err != nil {
		t.Fatalf("UpdateTransferStatus(completed) failed: %v", err)
	}

	completed, err := store.GetTransfer(ctx, transferCompleted.ID)
	if err != nil {
		t.Fatalf("GetTransfer(completed) failed: %v", err)
	}
	if completed == nil {
		t.Fatalf("GetTransfer(completed) returned nil")
	}
	if completed.Status != relayer.TransferStatusCompleted {
		t.Fatalf("unexpected completed status: got %s want %s", completed.Status, relayer.TransferStatusCompleted)
	}
	if completed.DestinationTxHash == nil || *completed.DestinationTxHash != destTxHash {
		t.Fatalf("unexpected destination tx hash: got %v want %s", completed.DestinationTxHash, destTxHash)
	}
	if completed.CompletedAt == nil {
		t.Fatalf("expected completed_at to be set")
	}

	transferFailed := &relayer.Transfer{
		ID:                "lifecycle-failed",
		Direction:         relayer.DirectionCantonToEthereum,
		Status:            relayer.TransferStatusPending,
		SourceChain:       relayer.ChainCanton,
		DestinationChain:  relayer.ChainEthereum,
		SourceTxHash:      "0xsource2",
		TokenAddress:      "0xtoken2",
		Amount:            "200",
		Sender:            "sender-2",
		Recipient:         "recipient-2",
		Nonce:             2,
		SourceBlockNumber: 20,
	}
	inserted, err = store.CreateTransfer(ctx, transferFailed)
	if err != nil {
		t.Fatalf("CreateTransfer(failed seed) failed: %v", err)
	}
	if !inserted {
		t.Fatalf("CreateTransfer(failed seed) expected inserted=true")
	}

	errMsg := "submit failed"
	if err = store.UpdateTransferStatus(
		ctx,
		transferFailed.ID,
		relayer.TransferStatusFailed,
		nil,
		&errMsg,
	); err != nil {
		t.Fatalf("UpdateTransferStatus(failed) failed: %v", err)
	}

	failed, err := store.GetTransfer(ctx, transferFailed.ID)
	if err != nil {
		t.Fatalf("GetTransfer(failed) failed: %v", err)
	}
	if failed == nil {
		t.Fatalf("GetTransfer(failed) returned nil")
	}
	if failed.Status != relayer.TransferStatusFailed {
		t.Fatalf("unexpected failed status: got %s want %s", failed.Status, relayer.TransferStatusFailed)
	}
	if failed.ErrorMessage == nil || *failed.ErrorMessage != errMsg {
		t.Fatalf("unexpected error message: got %v want %s", failed.ErrorMessage, errMsg)
	}
	if failed.CompletedAt != nil {
		t.Fatalf("expected completed_at to be nil for failed transfer")
	}

	if err = store.IncrementRetryCount(ctx, transferFailed.ID); err != nil {
		t.Fatalf("IncrementRetryCount(first) failed: %v", err)
	}
	if err = store.IncrementRetryCount(ctx, transferFailed.ID); err != nil {
		t.Fatalf("IncrementRetryCount(second) failed: %v", err)
	}
	failed, err = store.GetTransfer(ctx, transferFailed.ID)
	if err != nil {
		t.Fatalf("GetTransfer(after retries) failed: %v", err)
	}
	if failed.RetryCount != 2 {
		t.Fatalf("unexpected retry count: got %d want 2", failed.RetryCount)
	}

	err = store.UpdateTransferStatus(ctx, "missing-id", relayer.TransferStatusFailed, nil, &errMsg)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("UpdateTransferStatus(missing) expected not found error, got: %v", err)
	}

	err = store.IncrementRetryCount(ctx, "missing-id")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("IncrementRetryCount(missing) expected not found error, got: %v", err)
	}
}

func TestPGStore_TransferQueries(t *testing.T) {
	ctx, store := setupRelayerStore(t)

	base := time.Now().Add(-1 * time.Hour).UTC()
	seedTransfers := []TransferDao{
		{
			ID:                "seed-1",
			Direction:         string(relayer.DirectionCantonToEthereum),
			Status:            string(relayer.TransferStatusPending),
			SourceChain:       relayer.ChainCanton,
			DestinationChain:  relayer.ChainEthereum,
			SourceTxHash:      "0xseed1",
			TokenAddress:      "0xtoken",
			Amount:            "1",
			Sender:            "sender-1",
			Recipient:         "recipient-1",
			Nonce:             1,
			SourceBlockNumber: 101,
			CreatedAt:         base.Add(1 * time.Minute),
			UpdatedAt:         base.Add(1 * time.Minute),
		},
		{
			ID:                "seed-2",
			Direction:         string(relayer.DirectionCantonToEthereum),
			Status:            string(relayer.TransferStatusCompleted),
			SourceChain:       relayer.ChainCanton,
			DestinationChain:  relayer.ChainEthereum,
			SourceTxHash:      "0xseed2",
			TokenAddress:      "0xtoken",
			Amount:            "2",
			Sender:            "sender-2",
			Recipient:         "recipient-2",
			Nonce:             2,
			SourceBlockNumber: 102,
			CreatedAt:         base.Add(2 * time.Minute),
			UpdatedAt:         base.Add(2 * time.Minute),
		},
		{
			ID:                "seed-3",
			Direction:         string(relayer.DirectionCantonToEthereum),
			Status:            string(relayer.TransferStatusPending),
			SourceChain:       relayer.ChainCanton,
			DestinationChain:  relayer.ChainEthereum,
			SourceTxHash:      "0xseed3",
			TokenAddress:      "0xtoken",
			Amount:            "3",
			Sender:            "sender-3",
			Recipient:         "recipient-3",
			Nonce:             3,
			SourceBlockNumber: 103,
			CreatedAt:         base.Add(3 * time.Minute),
			UpdatedAt:         base.Add(3 * time.Minute),
		},
		{
			ID:                "seed-4",
			Direction:         string(relayer.DirectionEthereumToCanton),
			Status:            string(relayer.TransferStatusPending),
			SourceChain:       relayer.ChainEthereum,
			DestinationChain:  relayer.ChainCanton,
			SourceTxHash:      "0xseed4",
			TokenAddress:      "0xtoken",
			Amount:            "4",
			Sender:            "sender-4",
			Recipient:         "recipient-4",
			Nonce:             4,
			SourceBlockNumber: 104,
			CreatedAt:         base.Add(4 * time.Minute),
			UpdatedAt:         base.Add(4 * time.Minute),
		},
	}
	for i := range seedTransfers {
		if _, err := store.db.NewInsert().Model(&seedTransfers[i]).Exec(ctx); err != nil {
			t.Fatalf("failed to insert seed transfer %s: %v", seedTransfers[i].ID, err)
		}
	}

	pendingC2E, err := store.GetPendingTransfers(ctx, relayer.DirectionCantonToEthereum)
	if err != nil {
		t.Fatalf("GetPendingTransfers(canton->ethereum) failed: %v", err)
	}
	if len(pendingC2E) != 2 {
		t.Fatalf("unexpected pending transfer count: got %d want 2", len(pendingC2E))
	}
	if pendingC2E[0].ID != "seed-1" || pendingC2E[1].ID != "seed-3" {
		t.Fatalf("unexpected pending transfer order: got [%s, %s] want [seed-1, seed-3]", pendingC2E[0].ID, pendingC2E[1].ID)
	}

	latestThree, err := store.ListTransfers(ctx, 3)
	if err != nil {
		t.Fatalf("ListTransfers(limit=3) failed: %v", err)
	}
	if len(latestThree) != 3 {
		t.Fatalf("unexpected list length: got %d want 3", len(latestThree))
	}
	if latestThree[0].ID != "seed-4" || latestThree[1].ID != "seed-3" || latestThree[2].ID != "seed-2" {
		t.Fatalf("unexpected list order: got [%s, %s, %s] want [seed-4, seed-3, seed-2]",
			latestThree[0].ID, latestThree[1].ID, latestThree[2].ID)
	}
}

func TestPGStore_ChainState(t *testing.T) {
	ctx, store := setupRelayerStore(t)

	state, err := store.GetChainState(ctx, relayer.ChainCanton)
	if err != nil {
		t.Fatalf("GetChainState(missing) failed: %v", err)
	}
	if state != nil {
		t.Fatalf("GetChainState(missing) expected nil, got %+v", state)
	}

	if err = store.SetChainState(ctx, relayer.ChainCanton, 10, "offset-10"); err != nil {
		t.Fatalf("SetChainState(insert) failed: %v", err)
	}

	state, err = store.GetChainState(ctx, relayer.ChainCanton)
	if err != nil {
		t.Fatalf("GetChainState(after insert) failed: %v", err)
	}
	if state == nil {
		t.Fatalf("GetChainState(after insert) returned nil")
	}
	if state.LastBlock != 10 || state.Offset != "offset-10" {
		t.Fatalf("unexpected chain state after insert: got {LastBlock:%d Offset:%s}", state.LastBlock, state.Offset)
	}

	if err = store.SetChainState(ctx, relayer.ChainCanton, 42, "offset-42"); err != nil {
		t.Fatalf("SetChainState(update) failed: %v", err)
	}

	state, err = store.GetChainState(ctx, relayer.ChainCanton)
	if err != nil {
		t.Fatalf("GetChainState(after update) failed: %v", err)
	}
	if state == nil {
		t.Fatalf("GetChainState(after update) returned nil")
	}
	if state.LastBlock != 42 || state.Offset != "offset-42" {
		t.Fatalf("unexpected chain state after update: got {LastBlock:%d Offset:%s}", state.LastBlock, state.Offset)
	}
}

func TestPGStore_SteppableTransfers(t *testing.T) {
	ctx, store := setupRelayerStore(t)

	seed := func(id, bridgeKey string, status relayer.TransferStatus, nextStepAt *time.Time) {
		t.Helper()
		inserted, err := store.CreateTransfer(ctx, &relayer.Transfer{
			ID:               id,
			BridgeKey:        bridgeKey,
			TokenSymbol:      "USDCX",
			Direction:        relayer.DirectionEthereumToCanton,
			Status:           status,
			SourceChain:      relayer.ChainEthereum,
			DestinationChain: relayer.ChainCanton,
			SourceTxHash:     "0x" + id,
			TokenAddress:     "0xtoken",
			Amount:           "1",
			Sender:           "s",
			Recipient:        "r",
			NextStepAt:       nextStepAt,
		})
		if err != nil || !inserted {
			t.Fatalf("seed %s failed: inserted=%v err=%v", id, inserted, err)
		}
	}

	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)

	seed("due-nil", "xreserve", relayer.TransferStatusPending, nil)
	seed("due-past", "xreserve", relayer.TransferStatusInProgress, &past)
	seed("not-due", "xreserve", relayer.TransferStatusInProgress, &future)
	seed("terminal", "xreserve", relayer.TransferStatusCompleted, nil)
	seed("other-bridge", "wayfinder", relayer.TransferStatusPending, nil)

	got, err := store.GetSteppableTransfers(ctx, []string{"xreserve"}, 10)
	if err != nil {
		t.Fatalf("GetSteppableTransfers failed: %v", err)
	}
	if len(got) != 2 {
		ids := make([]string, 0, len(got))
		for _, tr := range got {
			ids = append(ids, tr.ID)
		}
		t.Fatalf("got %d steppable transfers (%v), want 2", len(got), ids)
	}
	for _, tr := range got {
		if tr.ID != "due-nil" && tr.ID != "due-past" {
			t.Fatalf("unexpected steppable transfer %q", tr.ID)
		}
	}

	// Empty key list is a no-op, not a full-table scan.
	got, err = store.GetSteppableTransfers(ctx, nil, 10)
	if err != nil {
		t.Fatalf("GetSteppableTransfers(nil keys) failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetSteppableTransfers(nil keys) returned %d transfers, want 0", len(got))
	}
}

func TestPGStore_ApplyStep(t *testing.T) {
	ctx, store := setupRelayerStore(t)

	inserted, err := store.CreateTransfer(ctx, &relayer.Transfer{
		ID:               "step-1",
		BridgeKey:        "xreserve",
		Direction:        relayer.DirectionEthereumToCanton,
		Status:           relayer.TransferStatusPending,
		SourceChain:      relayer.ChainEthereum,
		DestinationChain: relayer.ChainCanton,
		SourceTxHash:     "0xstep1",
		TokenAddress:     "0xtoken",
		Amount:           "1",
		Sender:           "s",
		Recipient:        "r",
		Metadata:         map[string]string{"deposit_nonce": "7"},
	})
	if err != nil || !inserted {
		t.Fatalf("seed failed: inserted=%v err=%v", inserted, err)
	}

	next := time.Now().Add(time.Minute)
	err = store.ApplyStep(ctx, "step-1", relayer.StepResult{
		Status:   relayer.TransferStatusInProgress,
		Stage:    "awaiting_attestation",
		Metadata: map[string]string{"attestation_id": "att-9"},
	}, next)
	if err != nil {
		t.Fatalf("ApplyStep(in_progress) failed: %v", err)
	}

	tr, err := store.GetTransfer(ctx, "step-1")
	if err != nil || tr == nil {
		t.Fatalf("GetTransfer failed: %v", err)
	}
	if tr.Status != relayer.TransferStatusInProgress || tr.Stage != "awaiting_attestation" {
		t.Fatalf("status/stage = %s/%s, want in_progress/awaiting_attestation", tr.Status, tr.Stage)
	}
	// Metadata is merged, not replaced.
	if tr.Metadata["deposit_nonce"] != "7" || tr.Metadata["attestation_id"] != "att-9" {
		t.Fatalf("metadata merge failed: %+v", tr.Metadata)
	}
	if tr.NextStepAt == nil {
		t.Fatalf("NextStepAt should be set for non-terminal status")
	}

	destTx := "0xminted"
	err = store.ApplyStep(ctx, "step-1", relayer.StepResult{
		Status:     relayer.TransferStatusCompleted,
		Stage:      "minted",
		DestTxHash: &destTx,
	}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("ApplyStep(completed) failed: %v", err)
	}

	tr, err = store.GetTransfer(ctx, "step-1")
	if err != nil || tr == nil {
		t.Fatalf("GetTransfer failed: %v", err)
	}
	if tr.Status != relayer.TransferStatusCompleted {
		t.Fatalf("status = %s, want completed", tr.Status)
	}
	if tr.DestinationTxHash == nil || *tr.DestinationTxHash != destTx {
		t.Fatalf("destination tx hash not persisted")
	}
	if tr.CompletedAt == nil {
		t.Fatalf("CompletedAt should be set on completion")
	}
	if tr.NextStepAt != nil {
		t.Fatalf("NextStepAt should be cleared for terminal status")
	}

	if err = store.ApplyStep(ctx, "missing", relayer.StepResult{Status: relayer.TransferStatusCompleted}, time.Now()); err == nil {
		t.Fatalf("ApplyStep on missing transfer should fail")
	}
}

func TestPGStore_RecordStepError(t *testing.T) {
	ctx, store := setupRelayerStore(t)

	inserted, err := store.CreateTransfer(ctx, &relayer.Transfer{
		ID:               "err-1",
		BridgeKey:        "xreserve",
		Direction:        relayer.DirectionEthereumToCanton,
		Status:           relayer.TransferStatusPending,
		SourceChain:      relayer.ChainEthereum,
		DestinationChain: relayer.ChainCanton,
		SourceTxHash:     "0xerr1",
		TokenAddress:     "0xtoken",
		Amount:           "1",
		Sender:           "s",
		Recipient:        "r",
	})
	if err != nil || !inserted {
		t.Fatalf("seed failed: inserted=%v err=%v", inserted, err)
	}

	next := time.Now().Add(time.Minute)
	if err = store.RecordStepError(ctx, "err-1", "boom", next); err != nil {
		t.Fatalf("RecordStepError failed: %v", err)
	}
	if err = store.RecordStepError(ctx, "err-1", "boom again", next); err != nil {
		t.Fatalf("RecordStepError(second) failed: %v", err)
	}

	tr, err := store.GetTransfer(ctx, "err-1")
	if err != nil || tr == nil {
		t.Fatalf("GetTransfer failed: %v", err)
	}
	if tr.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", tr.RetryCount)
	}
	if tr.ErrorMessage == nil || *tr.ErrorMessage != "boom again" {
		t.Fatalf("ErrorMessage not persisted: %v", tr.ErrorMessage)
	}
	if tr.NextStepAt == nil {
		t.Fatalf("NextStepAt should be scheduled after a step error")
	}

	if err = store.RecordStepError(ctx, "missing", "x", next); err == nil {
		t.Fatalf("RecordStepError on missing transfer should fail")
	}
}
