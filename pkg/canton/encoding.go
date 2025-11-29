package canton

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
)

// EncodeMintProposalArgs encodes the arguments for the CreateMintProposal choice
func EncodeMintProposalArgs(req *MintProposalRequest) *lapi.Record {
	return &lapi.Record{
		Fields: []*lapi.RecordField{
			{
				Label: "recipient",
				Value: PartyValue(req.Recipient),
			},
			{
				Label: "amount",
				Value: NumericValue(req.Amount),
			},
			{
				Label: "txHash",
				Value: TextValue(req.Reference),
			},
		},
	}
}

// DecodeBurnEvent decodes Daml created event into BurnEvent
func DecodeBurnEvent(eventID, txID string, record *lapi.Record) (*BurnEvent, error) {
	fields := make(map[string]*lapi.Value)
	for _, field := range record.Fields {
		fields[field.Label] = field.Value
	}

	operator, err := extractParty(fields["operator"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract operator: %w", err)
	}

	owner, err := extractParty(fields["owner"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract owner: %w", err)
	}

	amount, err := extractNumeric(fields["amount"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract amount: %w", err)
	}

	destination, err := extractText(fields["destination"]) // EvmAddress is Text
	if err != nil {
		return nil, fmt.Errorf("failed to extract destination: %w", err)
	}

	reference, err := extractText(fields["reference"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract reference: %w", err)
	}

	// direction is an enum, but for now we might just ignore it or extract as variant if needed
	// Assuming we only listen to BurnEvent where direction is ToEvm

	return &BurnEvent{
		EventID:       eventID,
		TransactionID: txID,
		Operator:      operator,
		Owner:         owner,
		Amount:        amount,
		Destination:   destination,
		Reference:     reference,
	}, nil
}

// Helper encoding functions

func TextValue(s string) *lapi.Value {
	return &lapi.Value{
		Sum: &lapi.Value_Text{
			Text: s,
		},
	}
}

func PartyValue(s string) *lapi.Value {
	return &lapi.Value{
		Sum: &lapi.Value_Party{
			Party: s,
		},
	}
}

func Int64Value(n int64) *lapi.Value {
	return &lapi.Value{
		Sum: &lapi.Value_Int64{
			Int64: n,
		},
	}
}

func NumericValue(s string) *lapi.Value {
	return &lapi.Value{
		Sum: &lapi.Value_Numeric{
			Numeric: s,
		},
	}
}

func ContractIdValue(cid string) *lapi.Value {
	return &lapi.Value{
		Sum: &lapi.Value_ContractId{
			ContractId: cid,
		},
	}
}

// Helper extraction functions

func extractText(v *lapi.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if t, ok := v.Sum.(*lapi.Value_Text); ok {
		return t.Text, nil
	}
	return "", fmt.Errorf("not a text value")
}

func extractParty(v *lapi.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if p, ok := v.Sum.(*lapi.Value_Party); ok {
		return p.Party, nil
	}
	return "", fmt.Errorf("not a party value")
}

func extractInt64(v *lapi.Value) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("nil value")
	}
	if i, ok := v.Sum.(*lapi.Value_Int64); ok {
		return i.Int64, nil
	}
	return 0, fmt.Errorf("not an int64 value")
}

func extractNumeric(v *lapi.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if n, ok := v.Sum.(*lapi.Value_Numeric); ok {
		return n.Numeric, nil
	}
	return "", fmt.Errorf("not a numeric value")
}

// BigIntToDecimal converts big.Int to Daml decimal string
func BigIntToDecimal(amount *big.Int, decimals int) string {
	// Canton typically uses 10 decimal places
	// Format: amount / 10^decimals
	// This is a simplified implementation
	// In a real implementation, we should use a proper decimal library

	// For now, just return string representation if decimals is 0
	if decimals == 0 {
		return amount.String()
	}

	// TODO: Implement proper decimal formatting
	return amount.String()
}

// DecimalToBigInt converts Daml decimal string to big.Int
func DecimalToBigInt(decimal string, decimals int) (*big.Int, error) {
	// Simple implementation: remove dot and parse as int, then adjust scale
	// Note: This assumes the decimal string has correct number of decimal places or fewer
	// In production, use a proper decimal library

	parts := strings.Split(decimal, ".")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid decimal format: %s", decimal)
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}

	// Pad fractional part if needed (or truncate if too long - simplified)
	if len(fractionalPart) > decimals {
		// Truncate
		fractionalPart = fractionalPart[:decimals]
	} else {
		// Pad
		for len(fractionalPart) < decimals {
			fractionalPart += "0"
		}
	}

	combined := integerPart + fractionalPart
	result := new(big.Int)
	_, ok := result.SetString(combined, 10)
	if !ok {
		return nil, fmt.Errorf("invalid number: %s", combined)
	}

	return result, nil
}
