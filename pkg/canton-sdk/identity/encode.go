package identity

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton-sdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
)

func encodeFingerprintMappingCreate(issuer, userParty, fingerprint, evmAddress string) *lapiv2.Record {
	evmVal := values.None()
	if evmAddress != "" {
		evmVal = values.Optional(RecordValue(values.TextValue(evmAddress)))
	}
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: values.PartyValue(issuer)},
			{Label: "userParty", Value: values.PartyValue(userParty)},
			{Label: "fingerprint", Value: values.TextValue(fingerprint)},
			{Label: "evmAddress", Value: evmVal},
		},
	}
}

func RecordValue(fields ...*lapiv2.Value) *lapiv2.Value {
	recordFields := make([]*lapiv2.RecordField, len(fields))
	for i, f := range fields {
		recordFields[i] = &lapiv2.RecordField{Value: f}
	}
	return &lapiv2.Value{
		Sum: &lapiv2.Value_Record{
			Record: &lapiv2.Record{Fields: recordFields},
		},
	}
}
