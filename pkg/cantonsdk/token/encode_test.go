package token

import (
	"encoding/json"
	"testing"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeAnyValue(t *testing.T) {
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }

	tests := []struct {
		name    string
		input   AnyValue
		wantTag string
		check   func(t *testing.T, v *lapiv2.Value)
	}{
		{
			name:    "AV_ContractId",
			input:   AnyValue{Tag: "AV_ContractId", Value: raw(`"some-contract-id"`)},
			wantTag: "AV_ContractId",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_ContractId)
				assert.Equal(t, "some-contract-id", inner.ContractId)
			},
		},
		{
			name:    "AV_Text",
			input:   AnyValue{Tag: "AV_Text", Value: raw(`"hello"`)},
			wantTag: "AV_Text",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_Text)
				assert.Equal(t, "hello", inner.Text)
			},
		},
		{
			name:    "AV_Party",
			input:   AnyValue{Tag: "AV_Party", Value: raw(`"party::1234"`)},
			wantTag: "AV_Party",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_Party)
				assert.Equal(t, "party::1234", inner.Party)
			},
		},
		{
			name:    "AV_Bool",
			input:   AnyValue{Tag: "AV_Bool", Value: raw(`true`)},
			wantTag: "AV_Bool",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_Bool)
				assert.True(t, inner.Bool)
			},
		},
		{
			name:    "AV_Int",
			input:   AnyValue{Tag: "AV_Int", Value: raw(`42`)},
			wantTag: "AV_Int",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_Int64)
				assert.Equal(t, int64(42), inner.Int64)
			},
		},
		{
			name:    "AV_Decimal",
			input:   AnyValue{Tag: "AV_Decimal", Value: raw(`"123.456"`)},
			wantTag: "AV_Decimal",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_Numeric)
				assert.Equal(t, "123.456", inner.Numeric)
			},
		},
		{
			name: "AV_List empty",
			input: AnyValue{Tag: "AV_List", Value: raw(`[]`)},
			wantTag: "AV_List",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				inner := vr.Variant.Value.Sum.(*lapiv2.Value_List)
				assert.Empty(t, inner.List.Elements)
			},
		},
		{
			name: "AV_List with items",
			input: AnyValue{
				Tag:   "AV_List",
				Value: raw(`[{"tag":"AV_Text","value":"a"},{"tag":"AV_Text","value":"b"}]`),
			},
			wantTag: "AV_List",
			check: func(t *testing.T, v *lapiv2.Value) {
				vr := v.Sum.(*lapiv2.Value_Variant)
				elems := vr.Variant.Value.Sum.(*lapiv2.Value_List).List.Elements
				require.Len(t, elems, 2)
				assert.Equal(t, "AV_Text", elems[0].Sum.(*lapiv2.Value_Variant).Variant.Constructor)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeAnyValue(tt.input)
			require.NoError(t, err)
			vr, ok := got.Sum.(*lapiv2.Value_Variant)
			require.True(t, ok, "expected Variant sum type")
			assert.Equal(t, tt.wantTag, vr.Variant.Constructor)
			tt.check(t, got)
		})
	}
}

func TestEncodeAnyValueUnsupportedTag(t *testing.T) {
	_, err := encodeAnyValue(AnyValue{Tag: "AV_Unknown", Value: json.RawMessage(`null`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported AnyValue tag")
}

func TestEncodeChoiceContextRecord(t *testing.T) {
	ctx := AcceptChoiceContext{
		Values: map[string]AnyValue{
			"utility.digitalasset.com/transfer-rule": {
				Tag:   "AV_ContractId",
				Value: json.RawMessage(`"contract-abc"`),
			},
			"utility.digitalasset.com/sender-credentials": {
				Tag:   "AV_List",
				Value: json.RawMessage(`[]`),
			},
		},
	}

	got, err := encodeChoiceContextRecord(ctx)
	require.NoError(t, err)

	rec := got.Sum.(*lapiv2.Value_Record).Record
	require.Len(t, rec.Fields, 1)
	assert.Equal(t, "values", rec.Fields[0].Label)

	tm := rec.Fields[0].Value.Sum.(*lapiv2.Value_TextMap).TextMap
	require.Len(t, tm.Entries, 2)
	// entries are sorted by key
	assert.Equal(t, "utility.digitalasset.com/sender-credentials", tm.Entries[0].Key)
	assert.Equal(t, "utility.digitalasset.com/transfer-rule", tm.Entries[1].Key)
}
