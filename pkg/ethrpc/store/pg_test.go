package store

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/ethereum"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/pgutil"
	mghelper "github.com/chainsafe/canton-middleware/pkg/pgutil/migrations"
)

const testChainID = uint64(31337)

func setupEVMStore(t *testing.T) (*PGStore, *bun.DB) {
	t.Helper()
	requireDockerAccess(t)

	ctx := context.Background()
	db, cleanup := pgutil.SetupTestDB(t)
	t.Cleanup(cleanup)

	if err := mghelper.CreateSchema(ctx, db, &EvmTransactionDao{}, &EvmStateDao{}, &EvmLogDao{}, &MempoolEntryDao{}); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Seed the evm_state singleton (migration 8 does this in production).
	if _, err := db.NewInsert().
		Model(&EvmStateDao{ID: 1, LatestBlock: 0}).
		On("CONFLICT (id) DO NOTHING").
		Exec(ctx); err != nil {
		t.Fatalf("seed evm_state singleton: %v", err)
	}

	return NewStore(db), db
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

	t.Skip("docker daemon socket is not accessible; skipping testcontainer-backed ethrpc store tests")
}

func TestPGStore_BlockMeta(t *testing.T) {
	ctx := context.Background()
	store, _ := setupEVMStore(t)

	latest, err := store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		t.Fatalf("GetLatestEvmBlockNumber(initial) failed: %v", err)
	}
	if latest != 0 {
		t.Fatalf("unexpected initial latest block: got %d want 0", latest)
	}

	block1, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(1) failed: %v", err)
	}
	if block1.Number() != 1 {
		t.Fatalf("unexpected first block number: got %d want 1", block1.Number())
	}
	if !bytes.Equal(block1.Hash(), ethereum.ComputeBlockHash(testChainID, 1)) {
		t.Fatalf("unexpected first block hash")
	}
	if err = block1.Finalize(ctx); err != nil {
		t.Fatalf("Finalize(block1) failed: %v", err)
	}

	block2, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(2) failed: %v", err)
	}
	if block2.Number() != 2 {
		t.Fatalf("unexpected second block number: got %d want 2", block2.Number())
	}
	if !bytes.Equal(block2.Hash(), ethereum.ComputeBlockHash(testChainID, 2)) {
		t.Fatalf("unexpected second block hash")
	}
	if err = block2.Abort(ctx); err != nil {
		t.Fatalf("Abort(block2) failed: %v", err)
	}

	// Aborted block must not increment evm_state.latest_block — only Finalize persists the
	// counter. After block1 (finalized) and block2 (aborted), latest_block should be 1.
	latest, err = store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		t.Fatalf("GetLatestEvmBlockNumber(after abort) failed: %v", err)
	}
	if latest != 1 {
		t.Fatalf("unexpected latest block after abort: got %d want 1", latest)
	}
}

func TestPGStore_BlockRollback(t *testing.T) {
	ctx := context.Background()
	store, _ := setupEVMStore(t)

	block, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock failed: %v", err)
	}
	if err = block.Abort(ctx); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	// Second rollback must be a no-op
	if err = block.Abort(ctx); err != nil {
		t.Fatalf("second Rollback should be no-op: %v", err)
	}

	// With SELECT … FOR UPDATE, the block number is only persisted on Finalize.
	// An aborted block does NOT consume a number — latest_block stays at 0.
	latest, err := store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		t.Fatalf("GetLatestEvmBlockNumber failed: %v", err)
	}
	if latest != 0 {
		t.Fatalf("aborted block should not advance counter: got %d want 0", latest)
	}
}

func TestPGStore_Transactions(t *testing.T) {
	ctx := context.Background()
	store, db := setupEVMStore(t)

	fromA := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fromB := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Each block must be finalized before opening the next one — NewBlock holds an exclusive
	// row lock on evm_state for the lifetime of the transaction.

	block1, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(1) failed: %v", err)
	}
	tx1 := &ethrpc.EvmTransaction{
		TxHash:      []byte{0x01},
		FromAddress: fromA,
		ToAddress:   "0xcccccccccccccccccccccccccccccccccccccccc",
		Nonce:       0,
		Input:       []byte{0xaa, 0xbb},
		ValueWei:    "0",
		Status:      1,
		BlockNumber: block1.Number(),
		BlockHash:   block1.Hash(),
		TxIndex:     0,
		GasUsed:     21000,
	}
	if err = block1.AddEvmTransaction(ctx, tx1); err != nil {
		t.Fatalf("AddEvmTransaction(tx1) failed: %v", err)
	}
	if err = block1.Finalize(ctx); err != nil {
		t.Fatalf("Finalize(block1) failed: %v", err)
	}

	block2, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(2) failed: %v", err)
	}
	tx2 := &ethrpc.EvmTransaction{
		TxHash:      []byte{0x02},
		FromAddress: fromA,
		ToAddress:   "0xdddddddddddddddddddddddddddddddddddddddd",
		Nonce:       3,
		Input:       []byte{0x11, 0x22},
		ValueWei:    "0",
		Status:      1,
		BlockNumber: block2.Number(),
		BlockHash:   block2.Hash(),
		TxIndex:     0,
		GasUsed:     22000,
	}
	if err = block2.AddEvmTransaction(ctx, tx2); err != nil {
		t.Fatalf("AddEvmTransaction(tx2) failed: %v", err)
	}
	if err = block2.Finalize(ctx); err != nil {
		t.Fatalf("Finalize(block2) failed: %v", err)
	}

	block3, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(3) failed: %v", err)
	}
	tx3 := &ethrpc.EvmTransaction{
		TxHash:      []byte{0x03},
		FromAddress: fromB,
		ToAddress:   "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Nonce:       2,
		Input:       []byte{0x33},
		ValueWei:    "0",
		Status:      1,
		BlockNumber: block3.Number(),
		BlockHash:   block3.Hash(),
		TxIndex:     0,
		GasUsed:     23000,
	}
	if err = block3.AddEvmTransaction(ctx, tx3); err != nil {
		t.Fatalf("AddEvmTransaction(tx3) failed: %v", err)
	}
	if err = block3.Finalize(ctx); err != nil {
		t.Fatalf("Finalize(block3) failed: %v", err)
	}
	if err = block2.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block2) failed: %v", err)
	}

	// Duplicate add is idempotent (ON CONFLICT DO NOTHING).
	block1dup, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(dup) failed: %v", err)
	}
	tx1dup := *tx1
	tx1dup.BlockNumber = block1dup.Number()
	tx1dup.BlockHash = block1dup.Hash()
	if err = block1dup.AddEvmTransaction(ctx, &tx1dup); err != nil {
		t.Fatalf("AddEvmTransaction(tx1 dup) failed: %v", err)
	}
	if err = block1dup.Finalize(ctx); err != nil {
		t.Fatalf("Finalize(block1dup) failed: %v", err)
	}
	if err = block3.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block3) failed: %v", err)
	}

	countRows, err := db.NewSelect().
		Model((*EvmTransactionDao)(nil)).
		Where("tx_hash = ?", tx1.TxHash).
		Count(ctx)
	if err != nil {
		t.Fatalf("count tx rows failed: %v", err)
	}
	if countRows != 1 {
		t.Fatalf("duplicate tx insert should be ignored: got %d rows want 1", countRows)
	}

	gotTx1, err := store.GetEvmTransaction(ctx, tx1.TxHash)
	if err != nil {
		t.Fatalf("GetEvmTransaction(tx1) failed: %v", err)
	}
	if gotTx1 == nil {
		t.Fatalf("GetEvmTransaction(tx1) returned nil")
	}
	if gotTx1.Nonce != tx1.Nonce || gotTx1.BlockNumber != tx1.BlockNumber || gotTx1.TxIndex != tx1.TxIndex {
		t.Fatalf("transaction mismatch: got %+v want %+v", gotTx1, tx1)
	}

	missingTx, err := store.GetEvmTransaction(ctx, []byte{0xff})
	if err != nil {
		t.Fatalf("GetEvmTransaction(missing) failed: %v", err)
	}
	if missingTx != nil {
		t.Fatalf("GetEvmTransaction(missing) expected nil, got %+v", missingTx)
	}

	// fromA: nonces 0 and 3 → next nonce = MAX(0,3)+1 = 4
	nonceA, err := store.GetEvmTransactionCount(ctx, fromA)
	if err != nil {
		t.Fatalf("GetEvmTransactionCount(fromA) failed: %v", err)
	}
	if nonceA != 4 {
		t.Fatalf("unexpected nonce for fromA: got %d want 4", nonceA)
	}

	// fromB: nonce 2 → next nonce = MAX(2)+1 = 3
	nonceB, err := store.GetEvmTransactionCount(ctx, fromB)
	if err != nil {
		t.Fatalf("GetEvmTransactionCount(fromB) failed: %v", err)
	}
	if nonceB != 3 {
		t.Fatalf("unexpected nonce for fromB: got %d want 3", nonceB)
	}

	nonceMissing, err := store.GetEvmTransactionCount(ctx, "0xffffffffffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatalf("GetEvmTransactionCount(missing) failed: %v", err)
	}
	if nonceMissing != 0 {
		t.Fatalf("unexpected nonce for missing address: got %d want 0", nonceMissing)
	}

	blockNum, err := store.GetBlockNumberByHash(ctx, block2.Hash())
	if err != nil {
		t.Fatalf("GetBlockNumberByHash(existing) failed: %v", err)
	}
	if blockNum != tx2.BlockNumber {
		t.Fatalf("unexpected block number: got %d want %d", blockNum, tx2.BlockNumber)
	}

	missingBlockNum, err := store.GetBlockNumberByHash(ctx, []byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("GetBlockNumberByHash(missing) failed: %v", err)
	}
	if missingBlockNum != 0 {
		t.Fatalf("unexpected missing block number: got %d want 0", missingBlockNum)
	}
}

func TestPGStore_Logs(t *testing.T) {
	ctx := context.Background()
	store, db := setupEVMStore(t)

	addressA := []byte{0xaa}
	addressB := []byte{0xbb}
	topicA := []byte{0x01}
	topicB := []byte{0x02}
	topicC := []byte{0x03}

	block, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock failed: %v", err)
	}
	blockHash := block.Hash()
	blockNum := block.Number()

	log0 := &ethrpc.EvmLog{
		TxHash:      []byte{0x10},
		LogIndex:    0,
		Address:     addressA,
		Topics:      [][]byte{topicA, topicC},
		Data:        []byte{0xbe, 0xef},
		BlockNumber: blockNum,
		BlockHash:   blockHash,
		TxIndex:     2,
	}
	log1 := &ethrpc.EvmLog{
		TxHash:      []byte{0x10},
		LogIndex:    1,
		Address:     addressA,
		Topics:      [][]byte{topicA, topicB},
		Data:        []byte{0xde, 0xad},
		BlockNumber: blockNum,
		BlockHash:   blockHash,
		TxIndex:     2,
		Removed:     false,
	}
	log2 := &ethrpc.EvmLog{
		TxHash:      []byte{0x11},
		LogIndex:    0,
		Address:     addressB,
		Topics:      [][]byte{topicA},
		Data:        []byte{0xca},
		BlockNumber: blockNum,
		BlockHash:   blockHash,
		TxIndex:     0,
	}
	log3 := &ethrpc.EvmLog{
		TxHash:      []byte{0x12},
		LogIndex:    0,
		Address:     addressA,
		Topics:      [][]byte{topicC},
		Data:        []byte{0xfe},
		BlockNumber: blockNum,
		BlockHash:   blockHash,
		TxIndex:     1,
	}

	for _, l := range []*ethrpc.EvmLog{log1, log0, log2, log3} {
		if err = block.AddEvmLog(ctx, l); err != nil {
			t.Fatalf("AddEvmLog failed: %v", err)
		}
	}
	if err = block.Finalize(ctx); err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Duplicate add via a second block is idempotent (ON CONFLICT DO NOTHING).
	block2, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(dup) failed: %v", err)
	}
	log0dup := *log0
	log0dup.BlockNumber = block2.Number()
	log0dup.BlockHash = block2.Hash()
	if err = block2.AddEvmLog(ctx, &log0dup); err != nil {
		t.Fatalf("AddEvmLog(dup) failed: %v", err)
	}
	if err = block2.Finalize(ctx); err != nil {
		t.Fatalf("Finalize(dup) failed: %v", err)
	}

	countRows, err := db.NewSelect().
		Model((*EvmLogDao)(nil)).
		Where("tx_hash = ?", log0.TxHash).
		Where("log_index = ?", log0.LogIndex).
		Count(ctx)
	if err != nil {
		t.Fatalf("count log rows failed: %v", err)
	}
	if countRows != 1 {
		t.Fatalf("duplicate log insert should be ignored: got %d rows want 1", countRows)
	}

	_, err = store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		t.Fatalf("GetLatestEvmBlockNumber failed: %v", err)
	}

	logsByTx, err := store.GetEvmLogsByTxHash(ctx, log0.TxHash)
	if err != nil {
		t.Fatalf("GetEvmLogsByTxHash failed: %v", err)
	}
	if len(logsByTx) != 2 {
		t.Fatalf("unexpected tx log count: got %d want 2", len(logsByTx))
	}
	if logsByTx[0].LogIndex != 0 || logsByTx[1].LogIndex != 1 {
		t.Fatalf("logs are not sorted by index: %+v", logsByTx)
	}
	if len(logsByTx[0].Topics) != 2 {
		t.Fatalf("unexpected topics length for first tx log: got %d want 2", len(logsByTx[0].Topics))
	}

	logsByAddress, err := store.GetEvmLogs(ctx, addressA, nil, blockNum, blockNum)
	if err != nil {
		t.Fatalf("GetEvmLogs(address filter) failed: %v", err)
	}
	if len(logsByAddress) != 3 {
		t.Fatalf("unexpected address-filtered log count: got %d want 3", len(logsByAddress))
	}
	if logsByAddress[0].BlockNumber != blockNum || logsByAddress[0].LogIndex != 0 {
		t.Fatalf("unexpected first ordered log for address filter: %+v", logsByAddress[0])
	}

	logsByTopic, err := store.GetEvmLogs(ctx, nil, topicA, blockNum, blockNum)
	if err != nil {
		t.Fatalf("GetEvmLogs(topic filter) failed: %v", err)
	}
	if len(logsByTopic) != 3 {
		t.Fatalf("unexpected topic-filtered log count: got %d want 3", len(logsByTopic))
	}

	logsByAddressAndTopic, err := store.GetEvmLogs(ctx, addressA, topicA, blockNum, blockNum)
	if err != nil {
		t.Fatalf("GetEvmLogs(address+topic filter) failed: %v", err)
	}
	if len(logsByAddressAndTopic) != 2 {
		t.Fatalf("unexpected address+topic log count: got %d want 2", len(logsByAddressAndTopic))
	}

	logsByRange, err := store.GetEvmLogs(ctx, nil, nil, blockNum, blockNum)
	if err != nil {
		t.Fatalf("GetEvmLogs(block range filter) failed: %v", err)
	}
	if len(logsByRange) != 4 {
		t.Fatalf("unexpected block-range log count: got %d want 4", len(logsByRange))
	}
}

func TestPGStore_Mempool(t *testing.T) {
	ctx := context.Background()
	store, _ := setupEVMStore(t)

	entry1 := &ethrpc.MempoolEntry{
		TxHash:           []byte{0xaa, 0x01},
		FromAddress:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContractAddress:  "0xcccccccccccccccccccccccccccccccccccccccc",
		RecipientAddress: "0xdddddddddddddddddddddddddddddddddddddddd",
		Nonce:            0,
		Input:            []byte{0x01, 0x02},
		AmountData:       []byte{0x00, 0x64}, // 100
	}
	entry2 := &ethrpc.MempoolEntry{
		TxHash:           []byte{0xaa, 0x02},
		FromAddress:      "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ContractAddress:  "0xcccccccccccccccccccccccccccccccccccccccc",
		RecipientAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Nonce:            1,
		Input:            []byte{0x03, 0x04},
		AmountData:       []byte{0x00, 0xc8}, // 200
	}
	entry3 := &ethrpc.MempoolEntry{
		TxHash:           []byte{0xaa, 0x03},
		FromAddress:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContractAddress:  "0xcccccccccccccccccccccccccccccccccccccccc",
		RecipientAddress: "0xffffffffffffffffffffffffffffffffffffffff",
		Nonce:            2,
		Input:            []byte{0x05},
		AmountData:       []byte{0x01, 0x00}, // 256
	}

	// Insert three entries.
	for _, e := range []*ethrpc.MempoolEntry{entry1, entry2, entry3} {
		if err := store.InsertMempoolEntry(ctx, e); err != nil {
			t.Fatalf("InsertMempoolEntry(%x) failed: %v", e.TxHash, err)
		}
	}

	// Duplicate insert is a no-op (ON CONFLICT DO NOTHING).
	if err := store.InsertMempoolEntry(ctx, entry1); err != nil {
		t.Fatalf("duplicate InsertMempoolEntry should not fail: %v", err)
	}

	// All three start as pending.
	pending, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolPending)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(pending) failed: %v", err)
	}
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending entries, got %d", len(pending))
	}

	// Transition entry1 → completed, entry3 → failed with error message.
	if err = store.CompleteMempoolEntry(ctx, entry1.TxHash); err != nil {
		t.Fatalf("CompleteMempoolEntry(entry1) failed: %v", err)
	}
	if err = store.FailMempoolEntry(ctx, entry3.TxHash, "canton error: insufficient funds"); err != nil {
		t.Fatalf("FailMempoolEntry(entry3) failed: %v", err)
	}

	pending, err = store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolPending)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(pending after updates) failed: %v", err)
	}
	if len(pending) != 1 || !bytes.Equal(pending[0].TxHash, entry2.TxHash) {
		t.Fatalf("expected only entry2 pending, got %d entries", len(pending))
	}

	failed, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolFailed)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(failed) failed: %v", err)
	}
	if len(failed) != 1 || !bytes.Equal(failed[0].TxHash, entry3.TxHash) {
		t.Fatalf("expected only entry3 failed, got %d entries", len(failed))
	}

	// ClaimMempoolEntries: entry1 is completed; claiming it must atomically mark it mined
	// and return it. The block transaction commits via Finalize.
	block, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock for ClaimMempoolEntries failed: %v", err)
	}
	claimed, err := block.ClaimMempoolEntries(ctx)
	if err != nil {
		t.Fatalf("ClaimMempoolEntries failed: %v", err)
	}
	if len(claimed) != 1 || !bytes.Equal(claimed[0].TxHash, entry1.TxHash) {
		t.Fatalf("ClaimMempoolEntries: expected entry1, got %d entries", len(claimed))
	}
	if err = block.Finalize(ctx); err != nil {
		t.Fatalf("Finalize after ClaimMempoolEntries failed: %v", err)
	}

	mined, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolMined)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(mined) failed: %v", err)
	}
	if len(mined) != 1 || !bytes.Equal(mined[0].TxHash, entry1.TxHash) {
		t.Fatalf("expected entry1 mined after Finalize, got %d mined entries", len(mined))
	}

	// ClaimMempoolEntries inside an aborted block must NOT persist the status change.
	if err = store.CompleteMempoolEntry(ctx, entry2.TxHash); err != nil {
		t.Fatalf("CompleteMempoolEntry(entry2) failed: %v", err)
	}
	abortBlock, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock for aborted ClaimMempoolEntries failed: %v", err)
	}
	if _, err = abortBlock.ClaimMempoolEntries(ctx); err != nil {
		t.Fatalf("ClaimMempoolEntries (aborted block) failed: %v", err)
	}
	if err = abortBlock.Abort(ctx); err != nil {
		t.Fatalf("Abort failed: %v", err)
	}

	// entry2 must still be completed — the claim was rolled back with the block.
	stillCompleted, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolCompleted)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(completed after abort) failed: %v", err)
	}
	if len(stillCompleted) != 1 || !bytes.Equal(stillCompleted[0].TxHash, entry2.TxHash) {
		t.Fatalf("ClaimMempoolEntries in aborted block must not persist: got %d completed entries", len(stillCompleted))
	}
}

// TestPGStore_ConcurrentMiners verifies the store's behaviour under concurrent miner
// goroutines — the scenario expected in multi-instance deployments.
//
// NewBlock holds an exclusive row lock on evm_state (SELECT … FOR UPDATE) for the
// lifetime of the block transaction, which serialises miners at the database level.
// While miner A holds the lock and processes entries, miner B blocks at NewBlock.
// By the time B acquires the lock, A has already committed and sealed the entries
// as mined, so B's GetMempoolEntriesByStatus returns nothing and B aborts cleanly.
//
// The store must guarantee:
//
//  1. All completed entries end up in exactly one block — the winner's block.
//  2. Losing miners that find no work abort without persisting anything.
//  3. No miner goroutine returns an error.
//
// Run with -race to surface any data races in the store layer.
func TestPGStore_ConcurrentMiners(t *testing.T) {
	ctx := context.Background()
	store, db := setupEVMStore(t)

	// Seed completed mempool entries directly — InsertMempoolEntry always sets
	// status=pending, so we insert via the DAO to set status=completed up-front.
	const numEntries = 20
	for i := 0; i < numEntries; i++ {
		dao := &MempoolEntryDao{
			TxHash:           []byte{0xcc, byte(i)},
			FromAddress:      fmt.Sprintf("0x%040x", i),
			ContractAddress:  "0xcccccccccccccccccccccccccccccccccccccccc",
			RecipientAddress: fmt.Sprintf("0x%040x", i+100),
			Nonce:            uint64(i),
			Input:            []byte{byte(i)},
			AmountData:       []byte{0x01},
			Status:           string(ethrpc.MempoolCompleted),
		}
		if _, err := db.NewInsert().Model(dao).Exec(ctx); err != nil {
			t.Fatalf("seed mempool entry %d: %v", i, err)
		}
	}

	// Run three concurrent miners, each performing a full mine cycle.
	const numMiners = 3
	errs := make([]error, numMiners)
	var wg sync.WaitGroup
	for i := range numMiners {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			errs[id] = runMinerCycle(ctx, store)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("miner %d failed: %v", i, err)
		}
	}

	// 1. All entries landed in exactly one block — miners are serialised by the
	//    evm_state lock, so only the winning miner commits transactions.
	var txRows []EvmTransactionDao
	if err := db.NewSelect().Model(&txRows).Scan(ctx); err != nil {
		t.Fatalf("query evm_transactions: %v", err)
	}
	if len(txRows) != numEntries {
		t.Errorf("expected %d evm_transactions, got %d", numEntries, len(txRows))
	}
	blockNums := make(map[uint64]bool, numEntries)
	for _, row := range txRows {
		blockNums[row.BlockNumber] = true
	}
	if len(blockNums) != 1 {
		t.Errorf("all transactions must be in one block; found %d distinct block numbers: %v", len(blockNums), blockNums)
	}

	// 2. Every entry is sealed as mined — no entry left behind.
	var mempoolRows []MempoolEntryDao
	if err := db.NewSelect().Model(&mempoolRows).Scan(ctx); err != nil {
		t.Fatalf("query mempool: %v", err)
	}
	for _, row := range mempoolRows {
		if row.Status != string(ethrpc.MempoolMined) {
			t.Errorf("entry %x has status %q, want %q", row.TxHash, row.Status, ethrpc.MempoolMined)
		}
	}
}

// runMinerCycle simulates one miner iteration: claim a block, fetch completed entries,
// write transactions, seal mempool entries, and commit.
func runMinerCycle(ctx context.Context, store *PGStore) error {
	block, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		return fmt.Errorf("NewBlock: %w", err)
	}
	defer block.Abort(ctx) //nolint:errcheck

	entries, err := block.ClaimMempoolEntries(ctx)
	if err != nil {
		return fmt.Errorf("ClaimMempoolEntries: %w", err)
	}
	if len(entries) == 0 {
		return nil // nothing to mine; Abort via defer — block number not consumed
	}

	for i := range entries {
		e := &entries[i]
		evmTx := &ethrpc.EvmTransaction{
			TxHash:      e.TxHash,
			FromAddress: e.FromAddress,
			ToAddress:   e.ContractAddress,
			Nonce:       e.Nonce,
			Input:       e.Input,
			ValueWei:    "0",
			Status:      1,
			BlockNumber: block.Number(),
			BlockHash:   block.Hash(),
			TxIndex:     uint(i),
			GasUsed:     21000,
		}
		if err = block.AddEvmTransaction(ctx, evmTx); err != nil {
			return fmt.Errorf("AddEvmTransaction: %w", err)
		}
	}
	return block.Finalize(ctx)
}
