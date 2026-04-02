package miner

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"
	"github.com/chainsafe/canton-middleware/pkg/ethrpc/miner/mocks"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

const testChainID = uint64(31337)
const testGasLimit = uint64(21000)

func newTestMiner(store Store) *Miner {
	return New(store, testChainID, testGasLimit, time.Second, zap.NewNop())
}

func sampleEntry(txHash byte, from, contract, recipient string, nonce uint64, amount int64) ethrpc.MempoolEntry {
	return ethrpc.MempoolEntry{
		TxHash:           []byte{txHash},
		FromAddress:      from,
		ContractAddress:  contract,
		RecipientAddress: recipient,
		Nonce:            nonce,
		Input:            []byte{0xa9, 0x05, 0x9c, 0xbb},
		AmountData:       big.NewInt(amount).Bytes(),
		Status:           ethrpc.MempoolCompleted,
	}
}

// blockHash is the deterministic hash returned by all test PendingBlock mocks.
var blockHash = []byte{0xaa, 0xbb, 0xcc}

// setupBlock creates a PendingBlock mock preconfigured with Number() and Hash()
// and a deferred Abort expectation (safe no-op after Finalize).
func setupBlock(t *testing.T, number uint64) *mocks.PendingBlock {
	t.Helper()
	block := mocks.NewPendingBlock(t)
	block.EXPECT().Number().Return(number).Maybe()
	block.EXPECT().Hash().Return(blockHash).Maybe()
	block.EXPECT().Abort(mock.Anything).Return(nil).Maybe()
	return block
}

// ─── mine() tests ────────────────────────────────────────────────────────────

func TestMine_NoEntries_AbortsBlock(t *testing.T) {
	block := setupBlock(t, 1)
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return(nil, nil)

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	require.NoError(t, m.mine(context.Background()))

	// Finalize must NOT have been called.
	block.AssertNotCalled(t, "Finalize", mock.Anything)
}

func TestMine_SingleEntry_CommitsBlock(t *testing.T) {
	entry := sampleEntry(0x01,
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0xcccccccccccccccccccccccccccccccccccccccc",
		"0xdddddddddddddddddddddddddddddddddddddd",
		0, 42,
	)

	block := setupBlock(t, 5)
	block.EXPECT().AddEvmTransaction(mock.Anything, mock.MatchedBy(func(tx *ethrpc.EvmTransaction) bool {
		return tx.BlockNumber == 5 &&
			tx.FromAddress == entry.FromAddress &&
			tx.ToAddress == entry.ContractAddress &&
			tx.TxIndex == 0 &&
			tx.GasUsed == testGasLimit
	})).Return(nil).Once()
	block.EXPECT().AddEvmLog(mock.Anything, mock.Anything).Return(nil).Once()
	block.EXPECT().Finalize(mock.Anything).Return(nil).Once()
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return([]ethrpc.MempoolEntry{entry}, nil)

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	require.NoError(t, m.mine(context.Background()))
}

func TestMine_MultipleEntries_CorrectTxIndexAndHashes(t *testing.T) {
	entries := []ethrpc.MempoolEntry{
		sampleEntry(0x01, "0xaaaa000000000000000000000000000000000001",
			"0xcccc000000000000000000000000000000000001",
			"0xdddd000000000000000000000000000000000001", 0, 100),
		sampleEntry(0x02, "0xaaaa000000000000000000000000000000000002",
			"0xcccc000000000000000000000000000000000001",
			"0xdddd000000000000000000000000000000000002", 1, 200),
		sampleEntry(0x03, "0xaaaa000000000000000000000000000000000003",
			"0xcccc000000000000000000000000000000000001",
			"0xdddd000000000000000000000000000000000003", 2, 300),
	}

	block := setupBlock(t, 10)

	// Expect 3 transactions with sequential TxIndex.
	for i := range entries {
		idx := uint(i)
		block.EXPECT().AddEvmTransaction(mock.Anything, mock.MatchedBy(func(tx *ethrpc.EvmTransaction) bool {
			return tx.TxIndex == idx && tx.BlockNumber == 10
		})).Return(nil).Once()
		block.EXPECT().AddEvmLog(mock.Anything, mock.MatchedBy(func(log *ethrpc.EvmLog) bool {
			return log.TxIndex == idx
		})).Return(nil).Once()
	}
	block.EXPECT().Finalize(mock.Anything).Return(nil).Once()
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return(entries, nil)

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	require.NoError(t, m.mine(context.Background()))
}

// ─── error propagation tests ─────────────────────────────────────────────────

func TestMine_NewBlockError(t *testing.T) {
	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(nil, errors.New("lock contention"))

	m := newTestMiner(store)
	err := m.mine(context.Background())
	require.EqualError(t, err, "lock contention")
}

func TestMine_GetEntriesError(t *testing.T) {
	block := setupBlock(t, 1)
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return(nil, errors.New("db read failed"))

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	err := m.mine(context.Background())
	require.EqualError(t, err, "db read failed")
}

func TestMine_AddEvmTransactionError_ReturnsEarly(t *testing.T) {
	block := setupBlock(t, 1)
	block.EXPECT().AddEvmTransaction(mock.Anything, mock.Anything).Return(errors.New("insert tx failed"))

	entry := sampleEntry(0x01, "0xaaaa000000000000000000000000000000000001",
		"0xcccc000000000000000000000000000000000001",
		"0xdddd000000000000000000000000000000000001", 0, 100)
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return([]ethrpc.MempoolEntry{entry}, nil)

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	err := m.mine(context.Background())
	require.EqualError(t, err, "insert tx failed")
	block.AssertNotCalled(t, "Finalize", mock.Anything)
}

func TestMine_AddEvmLogError_ReturnsEarly(t *testing.T) {
	block := setupBlock(t, 1)
	block.EXPECT().AddEvmTransaction(mock.Anything, mock.Anything).Return(nil)
	block.EXPECT().AddEvmLog(mock.Anything, mock.Anything).Return(errors.New("insert log failed"))

	entry := sampleEntry(0x01, "0xaaaa000000000000000000000000000000000001",
		"0xcccc000000000000000000000000000000000001",
		"0xdddd000000000000000000000000000000000001", 0, 100)
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return([]ethrpc.MempoolEntry{entry}, nil)

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	err := m.mine(context.Background())
	require.EqualError(t, err, "insert log failed")
	block.AssertNotCalled(t, "Finalize", mock.Anything)
}

func TestMine_ClaimMempoolEntriesError_ReturnsEarly(t *testing.T) {
	block := setupBlock(t, 1)
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return(nil, errors.New("claim mempool entries failed"))

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	err := m.mine(context.Background())
	require.EqualError(t, err, "claim mempool entries failed")
	block.AssertNotCalled(t, "Finalize", mock.Anything)
}

func TestMine_FinalizeError(t *testing.T) {
	block := setupBlock(t, 1)
	block.EXPECT().AddEvmTransaction(mock.Anything, mock.Anything).Return(nil)
	block.EXPECT().AddEvmLog(mock.Anything, mock.Anything).Return(nil)
	block.EXPECT().Finalize(mock.Anything).Return(errors.New("commit failed"))

	entry := sampleEntry(0x01, "0xaaaa000000000000000000000000000000000001",
		"0xcccc000000000000000000000000000000000001",
		"0xdddd000000000000000000000000000000000001", 0, 100)
	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return([]ethrpc.MempoolEntry{entry}, nil)

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil)

	m := newTestMiner(store)
	err := m.mine(context.Background())
	require.EqualError(t, err, "commit failed")
}

// ─── buildTransferLog tests ──────────────────────────────────────────────────

func TestBuildTransferLog_CorrectTopicsAndData(t *testing.T) {
	fromHex := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	recipientHex := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	contractHex := "0xcccccccccccccccccccccccccccccccccccccccc"
	amount := big.NewInt(1_000_000)

	entry := &ethrpc.MempoolEntry{
		TxHash:           []byte{0x01},
		FromAddress:      fromHex,
		ContractAddress:  contractHex,
		RecipientAddress: recipientHex,
		AmountData:       amount.Bytes(),
	}

	block := mocks.NewPendingBlock(t)
	block.EXPECT().Number().Return(uint64(7)).Maybe()
	block.EXPECT().Hash().Return(blockHash).Maybe()

	log := buildTransferLog(entry, block, 3)

	assert.Equal(t, uint64(7), log.BlockNumber)
	assert.Equal(t, blockHash, log.BlockHash)
	assert.Equal(t, uint(3), log.TxIndex)
	assert.Equal(t, uint(3), log.LogIndex)
	assert.False(t, log.Removed)

	// Address = contract address.
	contractAddr := common.HexToAddress(contractHex)
	assert.Equal(t, contractAddr.Bytes(), log.Address)

	// Topics: [Transfer sig, from, to].
	require.Len(t, log.Topics, 3)
	assert.Equal(t, transferEventTopic.Bytes(), log.Topics[0])

	fromAddr := common.HexToAddress(fromHex)
	expectedFrom := common.BytesToHash(common.LeftPadBytes(fromAddr.Bytes(), 32))
	assert.Equal(t, expectedFrom.Bytes(), log.Topics[1])

	toAddr := common.HexToAddress(recipientHex)
	expectedTo := common.BytesToHash(common.LeftPadBytes(toAddr.Bytes(), 32))
	assert.Equal(t, expectedTo.Bytes(), log.Topics[2])

	// Data = amount padded to 32 bytes.
	expectedData := common.LeftPadBytes(amount.Bytes(), 32)
	assert.Equal(t, expectedData, log.Data)
}

func TestBuildTransferLog_LargeAmount(t *testing.T) {
	amount := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil) // 1e18

	entry := &ethrpc.MempoolEntry{
		TxHash:           []byte{0xff},
		FromAddress:      "0x0000000000000000000000000000000000000001",
		ContractAddress:  "0x0000000000000000000000000000000000000002",
		RecipientAddress: "0x0000000000000000000000000000000000000003",
		AmountData:       amount.Bytes(),
	}

	block := mocks.NewPendingBlock(t)
	block.EXPECT().Number().Return(uint64(1)).Maybe()
	block.EXPECT().Hash().Return(blockHash).Maybe()

	log := buildTransferLog(entry, block, 0)

	got := new(big.Int).SetBytes(log.Data)
	assert.Equal(t, 0, got.Cmp(amount), "round-trip amount mismatch: got %s, want %s", got, amount)
}

// ─── Start() lifecycle test ──────────────────────────────────────────────────

func TestStart_StopsOnContextCancel(t *testing.T) {
	block := mocks.NewPendingBlock(t)
	block.EXPECT().Number().Return(uint64(1)).Maybe()
	block.EXPECT().Hash().Return(blockHash).Maybe()
	block.EXPECT().Abort(mock.Anything).Return(nil).Maybe()

	block.EXPECT().ClaimMempoolEntries(mock.Anything).Return(nil, nil).Maybe()

	store := mocks.NewStore(t)
	store.EXPECT().NewBlock(mock.Anything, testChainID).Return(block, nil).Maybe()

	m := New(store, testChainID, testGasLimit, 10*time.Millisecond, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Start returned — success.
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}
