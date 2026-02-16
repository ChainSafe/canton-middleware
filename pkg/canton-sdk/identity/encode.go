package identity

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/canton-sdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/canton-sdk/values"
)

func encodeFingerprintMappingCreate(issuer, userParty, fingerprint, evmAddress string) *lapiv2.Record {
	return &lapiv2.Record{
		Fields: []*lapiv2.RecordField{
			{Label: "issuer", Value: values.PartyValue(issuer)},
			{Label: "userParty", Value: values.PartyValue(userParty)},
			{Label: "fingerprint", Value: values.TextValue(fingerprint)},
			{Label: "evmAddress", Value: values.TextValue(evmAddress)},
		},
	}
}
