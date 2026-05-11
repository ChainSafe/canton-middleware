package token

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func decodeHolding(ce *lapiv2.CreatedEvent) *Holding {
	fields := values.RecordToMap(ce.CreateArguments)

	// CIP56Holding shape: {issuer, owner, instrumentId{admin,id}, amount, meta, lock}.
	// Utility.Registry.Holding.V0.Holding shape: {operator, provider, registrar, owner,
	// instrument{source,id,scheme}, label, amount, lock} — no top-level meta/instrumentId.
	// The Splice HoldingV1 view derives instrumentId.admin from registrar and
	// instrumentId.id from instrument.id, which is what we mirror here.
	var admin, id, issuer, symbol string
	meta := map[string]string{}
	if fields["instrumentId"] != nil {
		admin, id = values.DecodeInstrumentId(fields["instrumentId"])
		meta = values.DecodeMetadata(fields["meta"])
		issuer = values.Party(fields["issuer"])
		symbol = meta[values.MetaKeySymbol]
	} else if fields["instrument"] != nil {
		admin = values.NestedPartyField(fields["instrument"], "source")
		id = values.NestedTextField(fields["instrument"], "id")
		issuer = values.Party(fields["registrar"])
		symbol = id
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
