package token

import (
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"
	"github.com/chainsafe/canton-middleware/pkg/cantonsdk/values"
)

func decodeHolding(ce *lapiv2.CreatedEvent) *Holding {
	fields := values.RecordToMap(ce.CreateArguments)
	meta := values.DecodeMetadata(fields["meta"])
	admin, id := values.DecodeInstrumentId(fields["instrumentId"])

	return &Holding{
		ContractID:      ce.ContractId,
		Issuer:          values.Party(fields["issuer"]),
		Owner:           values.Party(fields["owner"]),
		Amount:          values.Numeric(fields["amount"]),
		Symbol:          meta[values.MetaKeySymbol],
		InstrumentAdmin: admin,
		InstrumentID:    id,
		Locked:          !values.IsNone(fields["lock"]),
		Metadata:        meta,
	}
}

func decodeTokenTransferEvent(ce *lapiv2.CreatedEvent) *TokenTransferEvent {
	fields := values.RecordToMap(ce.CreateArguments)

	return &TokenTransferEvent{
		ContractID:      ce.ContractId,
		Issuer:          values.Party(fields["issuer"]),
		FromParty:       values.OptionalParty(fields["fromParty"]),
		ToParty:         values.OptionalParty(fields["toParty"]),
		Amount:          values.Numeric(fields["amount"]),
		TokenSymbol:     values.Text(fields["tokenSymbol"]),
		EventType:       values.Text(fields["eventType"]),
		Timestamp:       values.Timestamp(fields["timestamp"]),
		EvmTxHash:       values.OptionalText(fields["evmTxHash"]),
		EvmDestination:  values.OptionalText(fields["evmDestination"]),
		UserFingerprint: values.OptionalText(fields["userFingerprint"]),
		AuditObservers:  values.PartyList(fields["auditObservers"]),
	}
}
