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

// TextOK extracts a text value and reports whether the type matched.
func TextOK(v *lapiv2.Value) (string, bool) {
	if v == nil {
		return "", false
	}
	t, ok := v.Sum.(*lapiv2.Value_Text)
	if !ok {
		return "", false
	}
	return t.Text, true
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

// PartyOK extracts a party value and reports whether the type matched.
func PartyOK(v *lapiv2.Value) (string, bool) {
	if v == nil {
		return "", false
	}
	p, ok := v.Sum.(*lapiv2.Value_Party)
	if !ok {
		return "", false
	}
	return p.Party, true
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

// NumericOK extracts a numeric value and reports whether the type matched.
func NumericOK(v *lapiv2.Value) (string, bool) {
	if v == nil {
		return "", false
	}
	n, ok := v.Sum.(*lapiv2.Value_Numeric)
	if !ok {
		return "", false
	}
	return n.Numeric, true
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

// TimestampOK extracts a timestamp and reports whether the type matched.
func TimestampOK(v *lapiv2.Value) (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	t, ok := v.Sum.(*lapiv2.Value_Timestamp)
	if !ok {
		return time.Time{}, false
	}
	return time.UnixMicro(t.Timestamp), true
}

// NewtypeText extracts the inner Text from a DAML newtype encoded as a
// single-field Record (e.g. EvmAddress = EvmAddress { value : Text }).
// The DAML Ledger API v2 returns newtype values as Records in CreatedEvent
// payloads. Returns "" if v is nil, not a Record, or the first field is not Text.
func NewtypeText(v *lapiv2.Value) string {
	if v == nil {
		return ""
	}
	rec, ok := v.Sum.(*lapiv2.Value_Record)
	if !ok || rec.Record == nil || len(rec.Record.Fields) == 0 {
		return ""
	}
	return Text(rec.Record.Fields[0].Value)
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

// NestedTextFieldOK accesses a Text field within a nested DAML Record value and
// reports whether the lookup succeeded with the right type.
func NestedTextFieldOK(v *lapiv2.Value, field string) (string, bool) {
	return TextOK(RecordField(v)[field])
}

// NestedPartyField accesses a Party field within a nested DAML Record value.
// Use this for fields like instrumentId.admin.
// Returns "" when v is nil, not a Record, or the field is absent.
func NestedPartyField(v *lapiv2.Value, field string) string {
	return Party(RecordField(v)[field])
}

// NestedPartyFieldOK accesses a Party field within a nested DAML Record value and
// reports whether the lookup succeeded with the right type.
func NestedPartyFieldOK(v *lapiv2.Value, field string) (string, bool) {
	return PartyOK(RecordField(v)[field])
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
	// DA.TextMap.TextMap serializes as Value_TextMap
	if tm, ok := v.Sum.(*lapiv2.Value_TextMap); ok && tm.TextMap != nil {
		for _, e := range tm.TextMap.Entries {
			if e.GetKey() == key {
				return Text(e.GetValue())
			}
		}
		return ""
	}
	// DA.Map.Map serializes as Value_GenMap with Text keys
	if gm, ok := v.Sum.(*lapiv2.Value_GenMap); ok && gm.GenMap != nil {
		for _, e := range gm.GenMap.Entries {
			if Text(e.GetKey()) == key {
				return Text(e.GetValue())
			}
		}
	}
	return ""
}
