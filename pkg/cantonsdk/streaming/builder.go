package streaming

import (
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

// FieldValue is an opaque DAML value used to construct LedgerEvents.
// Use the Make* functions below to create values of each DAML type.
// This keeps callers free of any direct lapiv2 dependency.
type FieldValue struct{ v *lapiv2.Value }

// MakeTextField wraps a Go string as a DAML Text value.
func MakeTextField(s string) FieldValue {
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Text{Text: s}}}
}

// MakePartyField wraps a party ID string as a DAML Party value.
func MakePartyField(s string) FieldValue {
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Party{Party: s}}}
}

// MakeNumericField wraps a decimal string as a DAML Numeric value.
func MakeNumericField(s string) FieldValue {
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Numeric{Numeric: s}}}
}

// MakeTimestampField wraps a time.Time as a DAML Timestamp value.
func MakeTimestampField(t time.Time) FieldValue {
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Timestamp{Timestamp: t.UnixMicro()}}}
}

// MakeNoneField returns a DAML Optional None value.
func MakeNoneField() FieldValue {
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{}}}}
}

// MakeSomePartyField returns a DAML Optional(Party) Some value.
func MakeSomePartyField(party string) FieldValue {
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{
		Value: &lapiv2.Value{Sum: &lapiv2.Value_Party{Party: party}},
	}}}}
}

// MakeRecordField builds a DAML Record value from a map of sub-fields.
func MakeRecordField(fields map[string]FieldValue) FieldValue {
	rf := make([]*lapiv2.RecordField, 0, len(fields))
	for k, v := range fields {
		rf = append(rf, &lapiv2.RecordField{Label: k, Value: v.v})
	}
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Record{Record: &lapiv2.Record{Fields: rf}}}}
}

// MakeSomeRecordField wraps a record in a DAML Optional(Record) Some value.
func MakeSomeRecordField(fields map[string]FieldValue) FieldValue {
	inner := MakeRecordField(fields)
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_Optional{Optional: &lapiv2.Optional{Value: inner.v}}}}
}

// MakeTextMapField builds a DAML TextMap value from a Go string map.
func MakeTextMapField(entries map[string]string) FieldValue {
	es := make([]*lapiv2.TextMap_Entry, 0, len(entries))
	for k, v := range entries {
		es = append(es, &lapiv2.TextMap_Entry{Key: k, Value: &lapiv2.Value{Sum: &lapiv2.Value_Text{Text: v}}})
	}
	return FieldValue{&lapiv2.Value{Sum: &lapiv2.Value_TextMap{TextMap: &lapiv2.TextMap{Entries: es}}}}
}
