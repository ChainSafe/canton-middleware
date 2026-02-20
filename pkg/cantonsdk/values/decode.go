package values

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
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

// PartyList extracts list of parties.
func PartyList(v *lapiv2.Value) []string {
	if v == nil {
		return []string{}
	}
	pl := make([]string, 0)
	if list, ok := v.Sum.(*lapiv2.Value_List); ok {
		for _, element := range list.List.Elements {
			party := Party(element)
			if party != "" {
				pl = append(pl, party)
			}
		}
	}
	return pl
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

// IsNone returns true if the value is a Daml None (Optional with no value).
func IsNone(v *lapiv2.Value) bool {
	if v == nil {
		return true
	}
	opt, ok := v.Sum.(*lapiv2.Value_Optional)
	if !ok {
		return false
	}
	return opt.Optional == nil || opt.Optional.Value == nil
}

// Timestamp extracts timestamp.
func Timestamp(v *lapiv2.Value) time.Time {
	if v == nil {
		return time.Time{}
	}
	if t, ok := v.Sum.(*lapiv2.Value_Timestamp); ok {
		return time.UnixMicro(t.Timestamp)
	}
	return time.Time{}
}
