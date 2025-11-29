package canton

import (
	"fmt"
	"math/big"

	lapiv1 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v1"
	"github.com/shopspring/decimal"
)

// EncodeMintProposalArgs encodes the arguments for the CreateMintProposal choice
func EncodeMintProposalArgs(req *MintProposalRequest) *lapiv1.Record {
	return &lapiv1.Record{
		Fields: []*lapiv1.RecordField{
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
func DecodeBurnEvent(eventID, txID string, record *lapiv1.Record) (*BurnEvent, error) {
	fields := make(map[string]*lapiv1.Value)
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

func TextValue(s string) *lapiv1.Value {
	return &lapiv1.Value{
		Sum: &lapiv1.Value_Text{
			Text: s,
		},
	}
}

func PartyValue(s string) *lapiv1.Value {
	return &lapiv1.Value{
		Sum: &lapiv1.Value_Party{
			Party: s,
		},
	}
}

func Int64Value(n int64) *lapiv1.Value {
	return &lapiv1.Value{
		Sum: &lapiv1.Value_Int64{
			Int64: n,
		},
	}
}

func NumericValue(s string) *lapiv1.Value {
	return &lapiv1.Value{
		Sum: &lapiv1.Value_Numeric{
			Numeric: s,
		},
	}
}

func ContractIdValue(cid string) *lapiv1.Value {
	return &lapiv1.Value{
		Sum: &lapiv1.Value_ContractId{
			ContractId: cid,
		},
	}
}

// Helper extraction functions

func extractText(v *lapiv1.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if t, ok := v.Sum.(*lapiv1.Value_Text); ok {
		return t.Text, nil
	}
	return "", fmt.Errorf("not a text value")
}

func extractParty(v *lapiv1.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if p, ok := v.Sum.(*lapiv1.Value_Party); ok {
		return p.Party, nil
	}
	return "", fmt.Errorf("not a party value")
}

func extractInt64(v *lapiv1.Value) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("nil value")
	}
	if i, ok := v.Sum.(*lapiv1.Value_Int64); ok {
		return i.Int64, nil
	}
	return 0, fmt.Errorf("not an int64 value")
}

func extractNumeric(v *lapiv1.Value) (string, error) {
	if v == nil {
		return "", fmt.Errorf("nil value")
	}
	if n, ok := v.Sum.(*lapiv1.Value_Numeric); ok {
		return n.Numeric, nil
	}
	return "", fmt.Errorf("not a numeric value")
}

// BigIntToDecimal converts big.Int to Daml decimal string
func BigIntToDecimal(amount *big.Int, decimals int) string {
	d := decimal.NewFromBigInt(amount, int32(-decimals))
	return d.String()
}

// DecimalToBigInt converts Daml decimal string to big.Int
func DecimalToBigInt(s string, decimals int) (*big.Int, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid decimal format: %w", err)
	}
	// Multiply by 10^decimals to get the integer representation
	d = d.Mul(decimal.New(1, int32(decimals)))
	return d.BigInt(), nil
}
