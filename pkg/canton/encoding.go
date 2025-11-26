package canton

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/chainsafe/canton-middleware/pkg/canton/lapi"
)

// EncodeWithdrawalArgs encodes withdrawal request into Daml Value
func EncodeWithdrawalArgs(req *WithdrawalRequest) *lapi.Value {
	return &lapi.Value{
		Sum: &lapi.Value_Record{
			Record: &lapi.Record{
				Fields: []*lapi.RecordField{
					{Label: "ethTxHash", Value: TextValue(req.EthTxHash)},
					{Label: "ethSender", Value: TextValue(req.EthSender)},
					{Label: "recipient", Value: PartyValue(req.Recipient)},
					{Label: "amount", Value: NumericValue(req.Amount)},
					{Label: "nonce", Value: Int64Value(req.Nonce)},
					{Label: "ethChainId", Value: Int64Value(req.EthChainID)},
				},
			},
		},
	}
}

// DecodeDepositRequest decodes Daml created event into DepositRequest
func DecodeDepositRequest(eventID, txID string, record *lapi.Record) (*DepositRequest, error) {
	fields := make(map[string]*lapi.Value)
	for _, field := range record.Fields {
		fields[field.Label] = field.Value
	}

	depositor, err := extractParty(fields["depositor"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract depositor: %w", err)
	}

	amount, err := extractNumeric(fields["amount"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract amount: %w", err)
	}

	ethRecipient, err := extractText(fields["ethRecipient"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract ethRecipient: %w", err)
	}

	ethChainID, err := extractInt64(fields["ethChainId"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract ethChainId: %w", err)
	}

	clientNonce, err := extractText(fields["clientNonce"])
	if err != nil {
		return nil, fmt.Errorf("failed to extract clientNonce: %w", err)
	}

	// Extract token info
	tokenSymbol, err := extractText(fields["tokenSymbol"])
	if err != nil {
		// Fallback or error? For now, let's error if missing as it's required
		return nil, fmt.Errorf("failed to extract tokenSymbol: %w", err)
	}
	mode := AssetModeLockUnlock // TODO: extract from mode field

	return &DepositRequest{
		EventID:       eventID,
		TransactionID: txID,
		Depositor:     depositor,
		TokenSymbol:   tokenSymbol,
		Amount:        amount,
		EthChainID:    ethChainID,
		EthRecipient:  ethRecipient,
		Mode:          mode,
		ClientNonce:   clientNonce,
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
