package values

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
)

const MetaKeySymbol = "splice.chainsafe.io/symbol"

// MetaSymbol extracts token symbol from a Splice Metadata value.
// Splice Metadata is a Record { values : TextMap Text }.
func MetaSymbol(v *lapiv2.Value) string {
	m := DecodeMetadata(v)
	return m[MetaKeySymbol]
}

// DecodeMetadata extracts a Splice Metadata { values : TextMap Text } into a Go map.
func DecodeMetadata(v *lapiv2.Value) map[string]string {
	out := make(map[string]string)
	if v == nil {
		return out
	}
	rec, ok := v.Sum.(*lapiv2.Value_Record)
	if !ok || rec.Record == nil {
		return out
	}
	fields := RecordToMap(rec.Record)
	valuesField := fields["values"]
	if valuesField == nil {
		return out
	}

	return DecodeTextMap(valuesField)
}

// DecodeTextMap extracts a Daml TextMap Text value into a Go map.
func DecodeTextMap(v *lapiv2.Value) map[string]string {
	out := make(map[string]string)
	if v == nil {
		return out
	}
	tm, ok := v.Sum.(*lapiv2.Value_TextMap)
	if !ok || tm.TextMap == nil {
		return out
	}
	for _, entry := range tm.TextMap.Entries {
		out[entry.Key] = Text(entry.Value)
	}
	return out
}

// EncodeMetadata creates a Splice Metadata { values : TextMap Text } value.
func EncodeMetadata(kvs map[string]string) *lapiv2.Value {
	entries := make([]*lapiv2.TextMap_Entry, 0, len(kvs))
	for k, v := range kvs {
		entries = append(entries, &lapiv2.TextMap_Entry{
			Key:   k,
			Value: TextValue(v),
		})
	}

	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{
						Label: "values",
						Value: &lapiv2.Value{
							Sum: &lapiv2.Value_TextMap{
								TextMap: &lapiv2.TextMap{Entries: entries},
							},
						},
					},
				},
			},
		},
	}
}

// EmptyMetadata returns an empty Splice Metadata record.
func EmptyMetadata() *lapiv2.Value {
	return EncodeMetadata(map[string]string{})
}

// EncodeInstrumentId creates a Splice InstrumentId { admin : Party, id : Text } value.
func EncodeInstrumentId(admin, id string) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "admin", Value: PartyValue(admin)},
					{Label: "id", Value: TextValue(id)},
				},
			},
		},
	}
}

// EncodeExtraArgs creates a Splice ExtraArgs { context: ChoiceContext { values: {} }, meta: Metadata { values: {} } } value.
func EncodeExtraArgs() *lapiv2.Value {
	emptyChoiceContext := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{
						Label: "values",
						Value: &lapiv2.Value{
							Sum: &lapiv2.Value_TextMap{
								TextMap: &lapiv2.TextMap{Entries: []*lapiv2.TextMap_Entry{}},
							},
						},
					},
				},
			},
		},
	}

	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "context", Value: emptyChoiceContext},
					{Label: "meta", Value: EmptyMetadata()},
				},
			},
		},
	}
}

// DecodeInstrumentId extracts admin and id from a Splice InstrumentId record value.
func DecodeInstrumentId(v *lapiv2.Value) (admin, id string) {
	if v == nil {
		return "", ""
	}
	rec, ok := v.Sum.(*lapiv2.Value_Record)
	if !ok || rec.Record == nil {
		return "", ""
	}
	fields := RecordToMap(rec.Record)
	return Party(fields["admin"]), Text(fields["id"])
}

// ListValue creates a Daml List value from a slice of values.
func ListValue(elements []*lapiv2.Value) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_List{
			List: &lapiv2.List{Elements: elements},
		},
	}
}

// Int64Value creates a Daml Int64 value.
func Int64Value(v int64) *lapiv2.Value {
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Int64{
			Int64: v,
		},
	}
}
