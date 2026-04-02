package store

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
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
	if err = store.UpdateMempoolStatus(ctx, entry1.TxHash, ethrpc.MempoolCompleted, ""); err != nil {
		t.Fatalf("UpdateMempoolStatus(entry1, completed) failed: %v", err)
	}
	if err = store.UpdateMempoolStatus(ctx, entry3.TxHash, ethrpc.MempoolFailed, "canton error: insufficient funds"); err != nil {
		t.Fatalf("UpdateMempoolStatus(entry3, failed) failed: %v", err)
	}

	pending, err = store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolPending)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(pending after updates) failed: %v", err)
	}
	if len(pending) != 1 || !bytes.Equal(pending[0].TxHash, entry2.TxHash) {
		t.Fatalf("expected only entry2 pending, got %d entries", len(pending))
	}

	completed, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolCompleted)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(completed) failed: %v", err)
	}
	if len(completed) != 1 || !bytes.Equal(completed[0].TxHash, entry1.TxHash) {
		t.Fatalf("expected only entry1 completed, got %d entries", len(completed))
	}

	failed, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolFailed)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(failed) failed: %v", err)
	}
	if len(failed) != 1 || !bytes.Equal(failed[0].TxHash, entry3.TxHash) {
		t.Fatalf("expected only entry3 failed, got %d entries", len(failed))
	}

	// MarkMined: mine entry1 within a pending block. The mempool update must commit
	// atomically with the block — if Finalize succeeds, the entry is mined.
	block, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock for MarkMined failed: %v", err)
	}
	if err = block.MarkMined(ctx, [][]byte{entry1.TxHash}); err != nil {
		t.Fatalf("MarkMined failed: %v", err)
	}
	if err = block.Finalize(ctx); err != nil {
		t.Fatalf("Finalize after MarkMined failed: %v", err)
	}

	mined, err := store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolMined)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(mined) failed: %v", err)
	}
	if len(mined) != 1 || !bytes.Equal(mined[0].TxHash, entry1.TxHash) {
		t.Fatalf("expected entry1 mined after Finalize, got %d mined entries", len(mined))
	}

	// MarkMined inside an aborted block must NOT persist the status change.
	if err = store.UpdateMempoolStatus(ctx, entry2.TxHash, ethrpc.MempoolCompleted, ""); err != nil {
		t.Fatalf("UpdateMempoolStatus(entry2, completed) failed: %v", err)
	}
	abortBlock, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock for aborted MarkMined failed: %v", err)
	}
	if err = abortBlock.MarkMined(ctx, [][]byte{entry2.TxHash}); err != nil {
		t.Fatalf("MarkMined (aborted block) failed: %v", err)
	}
	if err = abortBlock.Abort(ctx); err != nil {
		t.Fatalf("Abort failed: %v", err)
	}

	// entry2 must still be completed — the MarkMined was rolled back with the block.
	completed, err = store.GetMempoolEntriesByStatus(ctx, ethrpc.MempoolCompleted)
	if err != nil {
		t.Fatalf("GetMempoolEntriesByStatus(completed after abort) failed: %v", err)
	}
	if len(completed) != 1 || !bytes.Equal(completed[0].TxHash, entry2.TxHash) {
		t.Fatalf("MarkMined in aborted block must not persist: got %d completed entries", len(completed))
	}
}
