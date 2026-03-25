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

	if err := mghelper.CreateSchema(ctx, db, &EvmTransactionDao{}, &EvmStateDao{}, &EvmLogDao{}); err != nil {
		t.Fatalf("failed to create schema: %v", err)
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
	if block1.BlockNumber() != 1 {
		t.Fatalf("unexpected first block number: got %d want 1", block1.BlockNumber())
	}
	if !bytes.Equal(block1.BlockHash(), ethereum.ComputeBlockHash(testChainID, 1)) {
		t.Fatalf("unexpected first block hash")
	}
	if err = block1.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block1) failed: %v", err)
	}

	block2, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(2) failed: %v", err)
	}
	if block2.BlockNumber() != 2 {
		t.Fatalf("unexpected second block number: got %d want 2", block2.BlockNumber())
	}
	if !bytes.Equal(block2.BlockHash(), ethereum.ComputeBlockHash(testChainID, 2)) {
		t.Fatalf("unexpected second block hash")
	}
	if err = block2.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block2) failed: %v", err)
	}

	latest, err = store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		t.Fatalf("GetLatestEvmBlockNumber(after allocations) failed: %v", err)
	}
	if latest != 2 {
		t.Fatalf("unexpected latest block: got %d want 2", latest)
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

	// Block counter should still be advanced after rollback, creating a gap.
	latest, err := store.GetLatestEvmBlockNumber(ctx)
	if err != nil {
		t.Fatalf("GetLatestEvmBlockNumber failed: %v", err)
	}
	if latest != 1 {
		t.Fatalf("block counter should advance even after rollback: got %d want 1", latest)
	}
}

func TestPGStore_Transactions(t *testing.T) {
	ctx := context.Background()
	store, db := setupEVMStore(t)

	fromA := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fromB := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	// Block 1: two transactions from fromA
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
		BlockNumber: block1.BlockNumber(),
		BlockHash:   block1.BlockHash(),
		TxIndex:     0,
		GasUsed:     21000,
	}
	tx2 := &ethrpc.EvmTransaction{
		TxHash:      []byte{0x02},
		FromAddress: fromA,
		ToAddress:   "0xdddddddddddddddddddddddddddddddddddddddd",
		Nonce:       3,
		Input:       []byte{0x11, 0x22},
		ValueWei:    "0",
		Status:      1,
		BlockNumber: block1.BlockNumber(),
		BlockHash:   block1.BlockHash(),
		TxIndex:     1,
		GasUsed:     22000,
	}
	if err = block1.SaveEvmTransaction(ctx, tx1); err != nil {
		t.Fatalf("SaveEvmTransaction(tx1) failed: %v", err)
	}
	if err = block1.SaveEvmTransaction(ctx, tx2); err != nil {
		t.Fatalf("SaveEvmTransaction(tx2) failed: %v", err)
	}
	if err = block1.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block1) failed: %v", err)
	}

	// Block 2: one transaction from fromB
	block2, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(2) failed: %v", err)
	}
	tx3 := &ethrpc.EvmTransaction{
		TxHash:      []byte{0x03},
		FromAddress: fromB,
		ToAddress:   "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Nonce:       2,
		Input:       []byte{0x33},
		ValueWei:    "0",
		Status:      1,
		BlockNumber: block2.BlockNumber(),
		BlockHash:   block2.BlockHash(),
		TxIndex:     0,
		GasUsed:     23000,
	}
	if err = block2.SaveEvmTransaction(ctx, tx3); err != nil {
		t.Fatalf("SaveEvmTransaction(tx3) failed: %v", err)
	}
	if err = block2.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block2) failed: %v", err)
	}

	// Duplicate insert must be ignored
	block3, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(3) failed: %v", err)
	}
	if err = block3.SaveEvmTransaction(ctx, tx1); err != nil {
		t.Fatalf("SaveEvmTransaction(tx1 duplicate) failed: %v", err)
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

	nonceA, err := store.GetEvmTransactionCount(ctx, fromA)
	if err != nil {
		t.Fatalf("GetEvmTransactionCount(fromA) failed: %v", err)
	}
	if nonceA != 4 {
		t.Fatalf("unexpected nonce for fromA: got %d want 4", nonceA)
	}

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

	blockNum, err := store.GetBlockNumberByHash(ctx, block2.BlockHash())
	if err != nil {
		t.Fatalf("GetBlockNumberByHash(existing) failed: %v", err)
	}
	if blockNum != block2.BlockNumber() {
		t.Fatalf("unexpected block number: got %d want %d", blockNum, block2.BlockNumber())
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

	// Block 1: two logs for the same tx (tx hash 0x10)
	block1, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(1) failed: %v", err)
	}
	log0 := &ethrpc.EvmLog{
		TxHash:      []byte{0x10},
		LogIndex:    0,
		Address:     addressA,
		Topics:      [][]byte{topicA, topicC},
		Data:        []byte{0xbe, 0xef},
		BlockNumber: block1.BlockNumber(),
		BlockHash:   block1.BlockHash(),
		TxIndex:     2,
	}
	log1 := &ethrpc.EvmLog{
		TxHash:      []byte{0x10},
		LogIndex:    1,
		Address:     addressA,
		Topics:      [][]byte{topicA, topicB},
		Data:        []byte{0xde, 0xad},
		BlockNumber: block1.BlockNumber(),
		BlockHash:   block1.BlockHash(),
		TxIndex:     2,
	}
	if err = block1.SaveEvmLog(ctx, log0); err != nil {
		t.Fatalf("SaveEvmLog(log0) failed: %v", err)
	}
	if err = block1.SaveEvmLog(ctx, log1); err != nil {
		t.Fatalf("SaveEvmLog(log1) failed: %v", err)
	}
	if err = block1.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block1) failed: %v", err)
	}

	// Block 2: log from addressB
	block2, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(2) failed: %v", err)
	}
	log2 := &ethrpc.EvmLog{
		TxHash:      []byte{0x11},
		LogIndex:    0,
		Address:     addressB,
		Topics:      [][]byte{topicA},
		Data:        []byte{0xca},
		BlockNumber: block2.BlockNumber(),
		BlockHash:   block2.BlockHash(),
		TxIndex:     0,
	}
	if err = block2.SaveEvmLog(ctx, log2); err != nil {
		t.Fatalf("SaveEvmLog(log2) failed: %v", err)
	}
	if err = block2.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block2) failed: %v", err)
	}

	// Block 3: log from addressA with topicC
	block3, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(3) failed: %v", err)
	}
	log3 := &ethrpc.EvmLog{
		TxHash:      []byte{0x12},
		LogIndex:    0,
		Address:     addressA,
		Topics:      [][]byte{topicC},
		Data:        []byte{0xfe},
		BlockNumber: block3.BlockNumber(),
		BlockHash:   block3.BlockHash(),
		TxIndex:     1,
	}
	if err = block3.SaveEvmLog(ctx, log3); err != nil {
		t.Fatalf("SaveEvmLog(log3) failed: %v", err)
	}
	if err = block3.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block3) failed: %v", err)
	}

	// Duplicate insert must be ignored
	block4, err := store.NewBlock(ctx, testChainID)
	if err != nil {
		t.Fatalf("NewBlock(4) failed: %v", err)
	}
	if err = block4.SaveEvmLog(ctx, log0); err != nil {
		t.Fatalf("SaveEvmLog(log0 duplicate) failed: %v", err)
	}
	if err = block4.Finalize(ctx); err != nil {
		t.Fatalf("Commit(block4) failed: %v", err)
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

	latestBlock, err := store.GetLatestEvmBlockNumber(ctx)
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

	logsByAddress, err := store.GetEvmLogs(ctx, addressA, nil, 0, latestBlock)
	if err != nil {
		t.Fatalf("GetEvmLogs(address filter) failed: %v", err)
	}
	if len(logsByAddress) != 3 {
		t.Fatalf("unexpected address-filtered log count: got %d want 3", len(logsByAddress))
	}
	if logsByAddress[0].BlockNumber != block1.BlockNumber() || logsByAddress[0].LogIndex != 0 {
		t.Fatalf("unexpected first ordered log for address filter: %+v", logsByAddress[0])
	}

	logsByTopic, err := store.GetEvmLogs(ctx, nil, topicA, 0, latestBlock)
	if err != nil {
		t.Fatalf("GetEvmLogs(topic filter) failed: %v", err)
	}
	if len(logsByTopic) != 3 {
		t.Fatalf("unexpected topic-filtered log count: got %d want 3", len(logsByTopic))
	}

	logsByAddressAndTopic, err := store.GetEvmLogs(ctx, addressA, topicA, 0, latestBlock)
	if err != nil {
		t.Fatalf("GetEvmLogs(address+topic filter) failed: %v", err)
	}
	if len(logsByAddressAndTopic) != 2 {
		t.Fatalf("unexpected address+topic log count: got %d want 2", len(logsByAddressAndTopic))
	}

	logsByRange, err := store.GetEvmLogs(ctx, nil, nil, block2.BlockNumber(), block2.BlockNumber())
	if err != nil {
		t.Fatalf("GetEvmLogs(block range filter) failed: %v", err)
	}
	if len(logsByRange) != 1 {
		t.Fatalf("unexpected block-range log count: got %d want 1", len(logsByRange))
	}
	if logsByRange[0].BlockNumber != block2.BlockNumber() {
		t.Fatalf("unexpected block number from block-range query: got %d want %d", logsByRange[0].BlockNumber, block2.BlockNumber())
	}
}
