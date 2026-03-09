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
