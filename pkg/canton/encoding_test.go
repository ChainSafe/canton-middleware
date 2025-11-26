package canton

import (
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
)

func TestEncodeWithdrawalArgs(t *testing.T) {
	req := &WithdrawalRequest{
		EthTxHash:   "0x123",
		EthSender:   "0xabc",
		Recipient:   "Alice",
		Amount:      "100.50",
		Nonce:       1,
		EthChainID:  5,
		TokenSymbol: "ETH",
	}

	val := EncodeWithdrawalArgs(req)

	if val == nil {
		t.Fatal("EncodeWithdrawalArgs returned nil")
	}

	record := val.GetRecord()
	if record == nil {
		t.Fatal("Expected Record value")
	}

	fields := make(map[string]*lapi.Value)
	for _, f := range record.Fields {
		fields[f.Label] = f.Value
	}

	if fields["ethTxHash"].GetText() != req.EthTxHash {
		t.Errorf("Expected ethTxHash %s, got %s", req.EthTxHash, fields["ethTxHash"].GetText())
	}
	if fields["ethSender"].GetText() != req.EthSender {
		t.Errorf("Expected ethSender %s, got %s", req.EthSender, fields["ethSender"].GetText())
	}
	if fields["recipient"].GetParty() != req.Recipient {
		t.Errorf("Expected recipient %s, got %s", req.Recipient, fields["recipient"].GetParty())
	}
	if fields["amount"].GetNumeric() != req.Amount {
		t.Errorf("Expected amount %s, got %s", req.Amount, fields["amount"].GetNumeric())
	}
	if fields["nonce"].GetInt64() != req.Nonce {
		t.Errorf("Expected nonce %d, got %d", req.Nonce, fields["nonce"].GetInt64())
	}
	if fields["ethChainId"].GetInt64() != req.EthChainID {
		t.Errorf("Expected ethChainId %d, got %d", req.EthChainID, fields["ethChainId"].GetInt64())
	}
}

func TestDecodeDepositRequest(t *testing.T) {
	record := &lapi.Record{
		Fields: []*lapi.RecordField{
			{Label: "depositor", Value: PartyValue("Alice")},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "ethRecipient", Value: TextValue("0xRecipient")},
			{Label: "ethChainId", Value: Int64Value(5)},
			{Label: "clientNonce", Value: TextValue("nonce-123")},
			{Label: "tokenSymbol", Value: TextValue("ETH")},
		},
	}

	deposit, err := DecodeDepositRequest("event-1", "tx-1", record)
	if err != nil {
		t.Fatalf("DecodeDepositRequest failed: %v", err)
	}

	if deposit.EventID != "event-1" {
		t.Errorf("Expected EventID event-1, got %s", deposit.EventID)
	}
	if deposit.TransactionID != "tx-1" {
		t.Errorf("Expected TransactionID tx-1, got %s", deposit.TransactionID)
	}
	if deposit.EthRecipient != "0xRecipient" {
		t.Errorf("Expected EthRecipient 0xRecipient, got %s", deposit.EthRecipient)
	}
	if deposit.TokenSymbol != "ETH" {
		t.Errorf("Expected TokenSymbol ETH, got %s", deposit.TokenSymbol)
	}
	if deposit.Amount != "50.00" {
		t.Errorf("Expected Amount 50.00, got %s", deposit.Amount)
	}
	if deposit.ClientNonce != "nonce-123" {
		t.Errorf("Expected ClientNonce nonce-123, got %s", deposit.ClientNonce)
	}
}

func TestHelperFunctions(t *testing.T) {
	// TextValue
	tv := TextValue("hello")
	if tv.GetText() != "hello" {
		t.Errorf("TextValue failed")
	}

	// PartyValue
	pv := PartyValue("Alice")
	if pv.GetParty() != "Alice" {
		t.Errorf("PartyValue failed")
	}

	// Int64Value
	iv := Int64Value(123)
	if iv.GetInt64() != 123 {
		t.Errorf("Int64Value failed")
	}

	// NumericValue
	nv := NumericValue("10.5")
	if nv.GetNumeric() != "10.5" {
		t.Errorf("NumericValue failed")
	}
}
