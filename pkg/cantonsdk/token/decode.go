package token

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func decodeHolding(ce *lapiv2.CreatedEvent) *Holding {
	fields := values.RecordToMap(ce.CreateArguments)

	// CIP56Holding uses {issuer, owner, instrumentId{admin,id}, amount, meta, lock}.
	// Utility.Registry.Holding.V0.Holding uses {operator, provider, registrar, owner,
	// instrument{source,id,scheme}, label, amount, lock} — no top-level meta or
	// instrumentId. Detect by which field is present.
	var admin, id, issuer, symbol string
	var meta map[string]string
	if fields["instrumentId"] != nil {
		admin, id = values.DecodeInstrumentId(fields["instrumentId"])
		meta = values.DecodeMetadata(fields["meta"])
		issuer = values.Party(fields["issuer"])
		symbol = meta[values.MetaKeySymbol]
	} else if fields["instrument"] != nil {
		admin, id = decodeRegistryInstrumentIdentifier(fields["instrument"])
		issuer = values.Party(fields["registrar"])
		symbol = id
		meta = map[string]string{}
	}

	return &Holding{
		ContractID:      ce.ContractId,
		Issuer:          issuer,
		Owner:           values.Party(fields["owner"]),
		Amount:          values.Numeric(fields["amount"]),
		Symbol:          symbol,
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Locked:          !values.IsNone(fields["lock"]),
		Metadata:        meta,
	}
}

// decodeRegistryInstrumentIdentifier extracts admin and id from a
// Utility.Registry.Holding.V0.Types.InstrumentIdentifier record (`source`/`id`/`scheme`).
// `source` is the registrar party; `scheme` is ignored — the Splice HoldingV1 view
// reconstructs the instrument id as `InstrumentId{admin=registrar, id=instrument.id}`.
func decodeRegistryInstrumentIdentifier(v *lapiv2.Value) (admin, id string) {
	if v == nil {
		return "", ""
	}
	rec, ok := v.Sum.(*lapiv2.Value_Record)
	if !ok || rec.Record == nil {
		return "", ""
	}
	fields := values.RecordToMap(rec.Record)
	return values.Party(fields["source"]), values.Text(fields["id"])
}

func decodeTokenTransferEvent(ce *lapiv2.CreatedEvent) *TokenTransferEvent {
	fields := values.RecordToMap(ce.CreateArguments)
	admin, id := values.DecodeInstrumentId(fields["instrumentId"])

	return &TokenTransferEvent{
		ContractID:      ce.ContractId,
		Issuer:          values.Party(fields["issuer"]),
		FromParty:       values.OptionalParty(fields["fromParty"]),
		ToParty:         values.OptionalParty(fields["toParty"]),
		Amount:          values.Numeric(fields["amount"]),
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Timestamp:       values.Timestamp(fields["timestamp"]),
		Meta:            values.DecodeOptionalMetadata(fields["meta"]),
		AuditObservers:  values.PartyList(fields["auditObservers"]),
	}
}
