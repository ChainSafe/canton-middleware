package engine

import (
	"testing"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/streaming"
	"github.com/chainsafe/canton-middleware/pkg/indexer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Shared test constants (accessible from processor_test.go, same package)
// ---------------------------------------------------------------------------

const (
	testContractID      = "contract-id-1"
	testInstrumentID    = "DEMO"
	testInstrumentAdmin = "issuer-party::abc123"
	testIssuer          = "issuer-party::abc123"
	testAmount          = "100.000000000000000000"
	testRecipient       = "recipient-party::def456"
	testSender          = "sender-party::ghi789"
)

// ---------------------------------------------------------------------------
// Test event / transaction builders
// ---------------------------------------------------------------------------

func makeTransferEvent(contractID string, fromParty, toParty streaming.FieldValue, extra map[string]streaming.FieldValue) *streaming.LedgerEvent {
	fields := map[string]streaming.FieldValue{
		"instrumentId": streaming.MakeRecordField(map[string]streaming.FieldValue{
			"id":    streaming.MakeTextField(testInstrumentID),
			"admin": streaming.MakePartyField(testInstrumentAdmin),
		}),
		"issuer":    streaming.MakePartyField(testIssuer),
		"fromParty": fromParty,
		"toParty":   toParty,
		"amount":    streaming.MakeNumericField(testAmount),
		"timestamp": streaming.MakeTimestampField(time.Unix(1_700_000_000, 0)),
		"meta":      streaming.MakeNoneField(),
	}
	for k, v := range extra {
		fields[k] = v
	}
	return streaming.NewLedgerEvent(contractID, "pkg-id", tokenTransferEventModule, tokenTransferEventEntity, true, fields)
}

func makeTx(offset int64, events ...*streaming.LedgerEvent) *streaming.LedgerTransaction {
	return &streaming.LedgerTransaction{
		UpdateID:      "update-" + string(rune('0'+offset)),
		Offset:        offset,
		EffectiveTime: time.Unix(1_700_000_000, 0),
		Events:        events,
	}
}

// decodeAll applies decode to every event in tx and collects successful results.
func decodeAll(decode func(*streaming.LedgerTransaction, *streaming.LedgerEvent) (*indexer.ParsedEvent, bool), tx *streaming.LedgerTransaction) []*indexer.ParsedEvent {
	var out []*indexer.ParsedEvent
	for _, ev := range tx.Events {
		if pe, ok := decode(tx, ev); ok {
			out = append(out, pe)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Decoder tests
// ---------------------------------------------------------------------------

func TestDecoder_FilterModeAll_Mint(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeNoneField(), streaming.MakeSomePartyField(testRecipient), nil)
	got := decodeAll(decode, makeTx(1, ev))

	require.Len(t, got, 1)
	pe := got[0]
	assert.Equal(t, indexer.EventMint, pe.EventType)
	assert.Nil(t, pe.FromPartyID)
	assert.Equal(t, testRecipient, *pe.ToPartyID)
	assert.Equal(t, testInstrumentID, pe.InstrumentID)
	assert.Equal(t, testInstrumentAdmin, pe.InstrumentAdmin)
	assert.Equal(t, testIssuer, pe.Issuer)
	assert.Equal(t, testAmount, pe.Amount)
	assert.Equal(t, testContractID, pe.ContractID)
	assert.Equal(t, int64(1), pe.LedgerOffset)
	assert.Equal(t, "update-1", pe.TxID)
	assert.Equal(t, time.Unix(1_700_000_000, 0), pe.EffectiveTime)
}

func TestDecoder_FilterModeAll_Burn(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeSomePartyField(testSender), streaming.MakeNoneField(), nil)
	got := decodeAll(decode, makeTx(2, ev))

	require.Len(t, got, 1)
	pe := got[0]
	assert.Equal(t, indexer.EventBurn, pe.EventType)
	assert.Equal(t, testSender, *pe.FromPartyID)
	assert.Nil(t, pe.ToPartyID)
}

func TestDecoder_FilterModeAll_Transfer(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeSomePartyField(testSender), streaming.MakeSomePartyField(testRecipient), nil)
	got := decodeAll(decode, makeTx(3, ev))

	require.Len(t, got, 1)
	pe := got[0]
	assert.Equal(t, indexer.EventTransfer, pe.EventType)
	assert.Equal(t, testSender, *pe.FromPartyID)
	assert.Equal(t, testRecipient, *pe.ToPartyID)
}

func TestDecoder_BothPartiesAbsent_Dropped(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeNoneField(), streaming.MakeNoneField(), nil)
	got := decodeAll(decode, makeTx(4, ev))

	assert.Empty(t, got)
}

func TestDecoder_SkipsArchivedEvent(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev := streaming.NewLedgerEvent(testContractID, "pkg-id", tokenTransferEventModule, tokenTransferEventEntity, false, nil)
	got := decodeAll(decode, makeTx(5, ev))

	assert.Empty(t, got)
}

func TestDecoder_SkipsWrongTemplate(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev := streaming.NewLedgerEvent(testContractID, "pkg-id", "OtherModule", "OtherEntity", true, map[string]streaming.FieldValue{})
	got := decodeAll(decode, makeTx(6, ev))

	assert.Empty(t, got)
}

func TestDecoder_FilterModeWhitelist_Allowed(t *testing.T) {
	allowed := []indexer.InstrumentKey{{Admin: testInstrumentAdmin, ID: testInstrumentID}}
	decode := NewTokenTransferDecoder(indexer.FilterModeWhitelist, allowed, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeNoneField(), streaming.MakeSomePartyField(testRecipient), nil)
	got := decodeAll(decode, makeTx(7, ev))

	require.Len(t, got, 1)
	assert.Equal(t, testInstrumentID, got[0].InstrumentID)
}

func TestDecoder_FilterModeWhitelist_Blocked_WrongAdmin(t *testing.T) {
	allowed := []indexer.InstrumentKey{{Admin: "other-issuer::xyz", ID: testInstrumentID}}
	decode := NewTokenTransferDecoder(indexer.FilterModeWhitelist, allowed, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeNoneField(), streaming.MakeSomePartyField(testRecipient), nil)
	got := decodeAll(decode, makeTx(8, ev))

	assert.Empty(t, got)
}

func TestDecoder_FilterModeWhitelist_Blocked_WrongID(t *testing.T) {
	allowed := []indexer.InstrumentKey{{Admin: testInstrumentAdmin, ID: "OTHER"}}
	decode := NewTokenTransferDecoder(indexer.FilterModeWhitelist, allowed, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeNoneField(), streaming.MakeSomePartyField(testRecipient), nil)
	got := decodeAll(decode, makeTx(9, ev))

	assert.Empty(t, got)
}

func TestDecoder_FilterModeWhitelist_MultipleKeys_MatchingPasses(t *testing.T) {
	allowed := []indexer.InstrumentKey{
		{Admin: "other-issuer::xyz", ID: "OTHER"},
		{Admin: testInstrumentAdmin, ID: testInstrumentID},
	}
	decode := NewTokenTransferDecoder(indexer.FilterModeWhitelist, allowed, zap.NewNop())

	ev := makeTransferEvent(testContractID, streaming.MakeNoneField(), streaming.MakeSomePartyField(testRecipient), nil)
	got := decodeAll(decode, makeTx(10, ev))

	require.Len(t, got, 1)
}

func TestDecoder_BridgeMetaExtracted(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	meta := streaming.MakeSomeRecordField(map[string]streaming.FieldValue{
		"values": streaming.MakeTextMapField(map[string]string{
			metaKeyExternalTxID:    "0xdeadbeef",
			metaKeyExternalAddress: "0xabc",
			metaKeyFingerprint:     "fp-1",
		}),
	})
	ev := makeTransferEvent(testContractID,
		streaming.MakeSomePartyField(testSender), streaming.MakeSomePartyField(testRecipient),
		map[string]streaming.FieldValue{"meta": meta},
	)
	got := decodeAll(decode, makeTx(11, ev))

	require.Len(t, got, 1)
	pe := got[0]
	require.NotNil(t, pe.ExternalTxID)
	assert.Equal(t, "0xdeadbeef", *pe.ExternalTxID)
	require.NotNil(t, pe.ExternalAddress)
	assert.Equal(t, "0xabc", *pe.ExternalAddress)
	require.NotNil(t, pe.Fingerprint)
	assert.Equal(t, "fp-1", *pe.Fingerprint)
}

func TestDecoder_BridgeMeta_NoneField_NilPointers(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	// meta = None → all bridge fields should be nil
	ev := makeTransferEvent(testContractID,
		streaming.MakeSomePartyField(testSender), streaming.MakeSomePartyField(testRecipient), nil,
	)
	got := decodeAll(decode, makeTx(12, ev))

	require.Len(t, got, 1)
	pe := got[0]
	assert.Nil(t, pe.ExternalTxID)
	assert.Nil(t, pe.ExternalAddress)
	assert.Nil(t, pe.Fingerprint)
}

func TestDecoder_MultipleEventsInTx_OnlyMatchingReturned(t *testing.T) {
	decode := NewTokenTransferDecoder(indexer.FilterModeAll, nil, zap.NewNop())

	ev1 := makeTransferEvent("c-1", streaming.MakeNoneField(), streaming.MakeSomePartyField(testRecipient), nil)
	ev2 := makeTransferEvent("c-2", streaming.MakeSomePartyField(testSender), streaming.MakeNoneField(), nil)
	ev3 := streaming.NewLedgerEvent("c-3", "pkg", "Other", "Template", true, map[string]streaming.FieldValue{})

	got := decodeAll(decode, makeTx(13, ev1, ev2, ev3))

	require.Len(t, got, 2)
	assert.Equal(t, "c-1", got[0].ContractID)
	assert.Equal(t, "c-2", got[1].ContractID)
}
