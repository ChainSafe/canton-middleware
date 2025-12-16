package canton

import (
	"testing"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

func TestDecodeWithdrawalEvent(t *testing.T) {
	// EvmAddress is encoded as a record with a "value" field
	evmAddressRecord := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "value", Value: TextValue("0xRecipient")},
				},
			},
		},
	}

	// Status is encoded as a variant
	statusVariant := &lapiv2.Value{
		Sum: &lapiv2.Value_Variant{
			Variant: &lapiv2.Variant{
				Constructor: "Pending",
			},
		},
	}

	record := &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: PartyValue("Issuer")},
			{Label: "userParty", Value: PartyValue("Alice")},
			{Label: "evmDestination", Value: evmAddressRecord},
			{Label: "amount", Value: NumericValue("100.00")},
			{Label: "fingerprint", Value: TextValue("fp-123")},
			{Label: "status", Value: statusVariant},
		},
	}

	withdrawal, err := DecodeWithdrawalEvent("event-1", "tx-1", "cid-1", record)
	if err != nil {
		t.Fatalf("DecodeWithdrawalEvent failed: %v", err)
	}

	if withdrawal.EventID != "event-1" {
		t.Errorf("Expected EventID event-1, got %s", withdrawal.EventID)
	}
	if withdrawal.TransactionID != "tx-1" {
		t.Errorf("Expected TransactionID tx-1, got %s", withdrawal.TransactionID)
	}
	if withdrawal.ContractID != "cid-1" {
		t.Errorf("Expected ContractID cid-1, got %s", withdrawal.ContractID)
	}
	if withdrawal.Issuer != "Issuer" {
		t.Errorf("Expected Issuer Issuer, got %s", withdrawal.Issuer)
	}
	if withdrawal.UserParty != "Alice" {
		t.Errorf("Expected UserParty Alice, got %s", withdrawal.UserParty)
	}
	if withdrawal.Amount != "100.00" {
		t.Errorf("Expected Amount 100.00, got %s", withdrawal.Amount)
	}
	if withdrawal.EvmDestination != "0xRecipient" {
		t.Errorf("Expected EvmDestination 0xRecipient, got %s", withdrawal.EvmDestination)
	}
	if withdrawal.Fingerprint != "fp-123" {
		t.Errorf("Expected Fingerprint fp-123, got %s", withdrawal.Fingerprint)
	}
	if withdrawal.Status != "Pending" {
		t.Errorf("Expected Status Pending, got %s", withdrawal.Status)
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
