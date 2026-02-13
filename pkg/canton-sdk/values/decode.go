package values

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2"
)

// Text extracts a text value.
func Text(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if t, ok := v.Sum.(*lapiv2.Value_Text); ok {
		return t.Text
	}
	return ""
}

// Party extracts a party value.
func Party(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if p, ok := v.Sum.(*lapiv2.Value_Party); ok {
		return p.Party
	}
	return ""
}

// Numeric extracts a numeric value as string.
func Numeric(v *lapiv2.Value) string {
	if v == nil {
		return "0"
	}
	if n, ok := v.Sum.(*lapiv2.Value_Numeric); ok {
		return n.Numeric
	}
	return "0"
}

// ContractID extracts a contract ID value.
func ContractID(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	if c, ok := v.Sum.(*lapiv2.Value_ContractId); ok {
		return c.ContractId
	}
	return ""
}
