package canton

import (
	"testing"

	lapiv1 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1"
)

func TestEncodeMintProposalArgs(t *testing.T) {
	req := &MintProposalRequest{
		Recipient: "Bob",
		Amount:    "100.0",
		Reference: "tx-hash",
	}

	record := EncodeMintProposalArgs(req)

	if len(record.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(record.Fields))
	}

	fields := make(map[string]*lapiv1.Value)
	for _, f := range record.Fields {
		fields[f.Label] = f.Value
	}

	if fields["recipient"].GetParty() != req.Recipient {
		t.Errorf("Expected recipient %s, got %s", req.Recipient, fields["recipient"].GetParty())
	}
	if fields["amount"].GetNumeric() != req.Amount {
		t.Errorf("Expected amount %s, got %s", req.Amount, fields["amount"].GetNumeric())
	}
	if fields["txHash"].GetText() != req.Reference {
		t.Errorf("Expected txHash %s, got %s", req.Reference, fields["txHash"].GetText())
	}
}

func TestDecodeBurnEvent(t *testing.T) {
	record := &lapiv1.Record{
		Fields: []*lapiv1.RecordField{
			{Label: "operator", Value: PartyValue("Alice")},
			{Label: "owner", Value: PartyValue("Bob")},
			{Label: "amount", Value: NumericValue("50.00")},
			{Label: "destination", Value: TextValue("0xRecipient")},
			{Label: "reference", Value: TextValue("ref-123")},
		},
	}

	burn, err := DecodeBurnEvent("event-1", "tx-1", record)
	if err != nil {
		t.Fatalf("DecodeBurnEvent failed: %v", err)
	}

	if burn.EventID != "event-1" {
		t.Errorf("Expected EventID event-1, got %s", burn.EventID)
	}
	if burn.TransactionID != "tx-1" {
		t.Errorf("Expected TransactionID tx-1, got %s", burn.TransactionID)
	}
	if burn.Operator != "Alice" {
		t.Errorf("Expected Operator Alice, got %s", burn.Operator)
	}
	if burn.Owner != "Bob" {
		t.Errorf("Expected Owner Bob, got %s", burn.Owner)
	}
	if burn.Amount != "50.00" {
		t.Errorf("Expected Amount 50.00, got %s", burn.Amount)
	}
	if burn.Destination != "0xRecipient" {
		t.Errorf("Expected Destination 0xRecipient, got %s", burn.Destination)
	}
	if burn.Reference != "ref-123" {
		t.Errorf("Expected Reference ref-123, got %s", burn.Reference)
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
