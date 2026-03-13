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

// OptionalText extracts a Daml Optional Text value. Returns "" for None.
func OptionalText(v *lapiv2.Value) string {
	if IsNone(v) {
		return ""
	}
	if opt, ok := v.Sum.(*lapiv2.Value_Optional); ok {
		return Text(opt.Optional.Value)
	}
	return ""
}

// OptionalParty extracts a Daml Optional Party value. Returns "" for None.
func OptionalParty(v *lapiv2.Value) string {
	if IsNone(v) {
		return ""
	}
	if opt, ok := v.Sum.(*lapiv2.Value_Optional); ok {
		return Party(opt.Optional.Value)
	}
	return ""
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

// RecordField extracts a named field from a Record value, returning the sub-map.
// Returns nil when v is nil or not a Record.
func RecordField(v *lapiv2.Value) map[string]*lapiv2.Value {
	if v == nil {
		return nil
	}
	r, ok := v.Sum.(*lapiv2.Value_Record)
	if !ok || r.Record == nil {
		return nil
	}
	return RecordToMap(r.Record)
}

// NestedTextField accesses a Text field within a nested DAML Record value.
// Use this for fields like instrumentId.id where instrumentId is a Record.
// Returns "" when v is nil, not a Record, or the field is absent.
func NestedTextField(v *lapiv2.Value, field string) string {
	return Text(RecordField(v)[field])
}

// NestedPartyField accesses a Party field within a nested DAML Record value.
// Use this for fields like instrumentId.admin.
// Returns "" when v is nil, not a Record, or the field is absent.
func NestedPartyField(v *lapiv2.Value, field string) string {
	return Party(RecordField(v)[field])
}

// OptionalRecordFields extracts the inner Record fields from an Optional(Record) value.
// Returns nil when v is None or the inner value is not a Record.
func OptionalRecordFields(v *lapiv2.Value) map[string]*lapiv2.Value {
	if IsNone(v) {
		return nil
	}
	opt, ok := v.Sum.(*lapiv2.Value_Optional)
	if !ok || opt.Optional == nil || opt.Optional.Value == nil {
		return nil
	}
	return RecordField(opt.Optional.Value)
}

// MapLookupText looks up a string key in a DAML Map Text Text value.
// Handles both TextMap (DA.TextMap) and GenMap (DA.Map) encodings.
// Returns "" when v is nil, not a map, or the key is absent.
func MapLookupText(v *lapiv2.Value, key string) string {
	if v == nil {
		return ""
	}
	// DA.TextMap.TextMap serialises as Value_TextMap
	if tm, ok := v.Sum.(*lapiv2.Value_TextMap); ok && tm.TextMap != nil {
		for _, e := range tm.TextMap.Entries {
			if e.GetKey() == key {
				return Text(e.GetValue())
			}
		}
		return ""
	}
	// DA.Map.Map serialises as Value_GenMap with Text keys
	if gm, ok := v.Sum.(*lapiv2.Value_GenMap); ok && gm.GenMap != nil {
		for _, e := range gm.GenMap.Entries {
			if Text(e.GetKey()) == key {
				return Text(e.GetValue())
			}
		}
	}
	return ""
}
