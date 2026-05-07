package token

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func encodeIssuerMintArgs(req *MintRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "recipient", Value: values.PartyValue(req.RecipientParty)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
			{Label: "eventMeta", Value: values.EncodeOptionalMetadata(req.EventMeta)},
		},
	}
}

func encodeIssuerBurnArgs(req *BurnRequest) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "holdingCid", Value: values.ContractIDValue(req.HoldingCID)},
			{Label: "amount", Value: values.NumericValue(req.Amount)},
			{Label: "eventTime", Value: values.TimestampValue(time.Now())},
			{Label: "eventMeta", Value: values.EncodeOptionalMetadata(req.EventMeta)},
		},
	}
}

// encodeAnyValue converts an AnyValue (JSON ADT) into a Daml-LF Variant value.
// Covers all AV_* tags used by the DA registrar protocol.
func encodeAnyValue(av AnyValue) (*lapiv2.Value, error) {
	var inner *lapiv2.Value
	var err error
	switch av.Tag {
	case "AV_ContractId":
		var s string
		if err = json.Unmarshal(av.Value, &s); err != nil {
			return nil, fmt.Errorf("AV_ContractId: %w", err)
		}
		inner = values.ContractIDValue(s)
	case "AV_Text":
		var s string
		if err = json.Unmarshal(av.Value, &s); err != nil {
			return nil, fmt.Errorf("AV_Text: %w", err)
		}
		inner = values.TextValue(s)
	case "AV_Party":
		var s string
		if err = json.Unmarshal(av.Value, &s); err != nil {
			return nil, fmt.Errorf("AV_Party: %w", err)
		}
		inner = values.PartyValue(s)
	case "AV_Bool":
		var b bool
		if err = json.Unmarshal(av.Value, &b); err != nil {
			return nil, fmt.Errorf("AV_Bool: %w", err)
		}
		inner = &lapiv2.Value{Sum: &lapiv2.Value_Bool{Bool: b}}
	case "AV_Int":
		var n json.Number
		if err = json.Unmarshal(av.Value, &n); err != nil {
			return nil, fmt.Errorf("AV_Int: %w", err)
		}
		var i int64
		if i, err = n.Int64(); err != nil {
			return nil, fmt.Errorf("AV_Int: %w", err)
		}
		inner = values.Int64Value(i)
	case "AV_Decimal":
		var s string
		if err = json.Unmarshal(av.Value, &s); err != nil {
			return nil, fmt.Errorf("AV_Decimal: %w", err)
		}
		inner = values.NumericValue(s)
	case "AV_List":
		inner, err = encodeAnyValueList(av.Value)
		if err != nil {
			return nil, err
		}
	case "AV_Map":
		inner, err = encodeAnyValueMap(av.Value)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported AnyValue tag %q", av.Tag)
	}
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Variant{
			Variant: &lapiv2.Variant{
				Constructor: av.Tag,
				Value:       inner,
			},
		},
	}, nil
}

func encodeAnyValueList(raw json.RawMessage) (*lapiv2.Value, error) {
	var items []AnyValue
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("AV_List: %w", err)
	}
	elems := make([]*lapiv2.Value, 0, len(items))
	for _, it := range items {
		v, err := encodeAnyValue(it)
		if err != nil {
			return nil, err
		}
		elems = append(elems, v)
	}
	return values.ListValue(elems), nil
}

func encodeAnyValueMap(raw json.RawMessage) (*lapiv2.Value, error) {
	// AV_Map is encoded as a list of {"_1": key, "_2": value} tuples.
	var items []struct {
		Key   string   `json:"_1"`
		Value AnyValue `json:"_2"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("AV_Map: %w", err)
	}
	elems := make([]*lapiv2.Value, 0, len(items))
	for _, it := range items {
		v, err := encodeAnyValue(it.Value)
		if err != nil {
			return nil, err
		}
		elems = append(elems, &lapiv2.Value{
			Sum: &lapiv2.Value_Record{
				Record: &lapiv2.Record{
					Fields: []*lapiv2.RecordField{
						{Label: "_1", Value: values.TextValue(it.Key)},
						{Label: "_2", Value: v},
					},
				},
			},
		})
	}
	return values.ListValue(elems), nil
}

// encodeChoiceContextRecord builds Splice ChoiceContext { values: TextMap AnyValue }.
func encodeChoiceContextRecord(ctx AcceptChoiceContext) (*lapiv2.Value, error) {
	keys := make([]string, 0, len(ctx.Values))
	for k := range ctx.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]*lapiv2.TextMap_Entry, 0, len(ctx.Values))
	for _, k := range keys {
		v, err := encodeAnyValue(ctx.Values[k])
		if err != nil {
			return nil, fmt.Errorf("encode key %q: %w", k, err)
		}
		entries = append(entries, &lapiv2.TextMap_Entry{Key: k, Value: v})
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
	}, nil
}

func encodeTransferFactoryTransferArgs(
	expectedAdmin string,
	sender string,
	receiver string,
	amount string,
	instrumentAdmin string,
	instrumentID string,
	requestedAt time.Time,
	executeBefore time.Time,
	inputHoldingCIDs []string,
	choiceContext map[string]string,
) *lapiv2.Record {
	holdingCidValues := make([]*lapiv2.Value, len(inputHoldingCIDs))
	for i, cid := range inputHoldingCIDs {
		holdingCidValues[i] = values.ContractIDValue(cid)
	}

	transfer := &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{
				Fields: []*lapiv2.RecordField{
					{Label: "sender", Value: values.PartyValue(sender)},
					{Label: "receiver", Value: values.PartyValue(receiver)},
					{Label: "amount", Value: values.NumericValue(amount)},
					{Label: "instrumentId", Value: values.EncodeInstrumentId(instrumentAdmin, instrumentID)},
					{Label: "requestedAt", Value: values.TimestampValue(requestedAt)},
					{Label: "executeBefore", Value: values.TimestampValue(executeBefore)},
					{Label: "inputHoldingCids", Value: values.ListValue(holdingCidValues)},
					{Label: "meta", Value: values.EmptyMetadata()},
				},
			},
		},
	}

	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "expectedAdmin", Value: values.PartyValue(expectedAdmin)},
			{Label: "transfer", Value: transfer},
			{Label: "extraArgs", Value: values.EncodeExtraArgs(choiceContext)},
		},
	}
}
